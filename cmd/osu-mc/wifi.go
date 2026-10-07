package main

import (
	"errors"
	"net/netip"
	"runtime"
	"sync"
	"time"

	"github.com/soypat/lneto"
	"github.com/soypat/lneto/x/xnet"
	"tinygo.org/x/espradio"

	"osu-mc/internal/application"
)

// wifiLink is the station interface: it scans and joins networks for the
// Wi-Fi page, and is the netdev that net/http dials through.
//
// It follows espradio's netlink.Esplink, which cannot be used as is: that
// enables and starts the radio on every NetConnect, so it only ever joins
// once, while the Wi-Fi page joins as often as a password is typed. Here the
// radio is brought up once, before the first scan or join, and the network
// stack once, after the first join; every join after that only associates
// and asks DHCP for a new address.
type wifiLink struct {
	// radio serializes the calls into the radio: a scan and a join must not
	// run at once.
	radio sync.Mutex

	up    sync.Once
	upErr error

	// mu guards what the UI goroutine reads while a join runs on another:
	// the status and the stack, which exists after the first join.
	mu        sync.Mutex
	state     application.WiFiStatus
	netstack  *espradio.Stack
	berkeley  *xnet.StackBerkeley
	stackOnce sync.Once
}

const (
	// The hostname keeps the dash: a DHCP hostname may only hold letters,
	// digits and '-', so "osu!mc" would be rejected or mangled.
	hostname = "osu-mc"

	// pollTime is how long the stack goroutine sleeps when there was
	// nothing to send or receive, and how long lookups wait between tries.
	pollTime = 5 * time.Millisecond
)

var pollBackoff = lneto.BackoffStrategy(func(uint) time.Duration { return pollTime })

var (
	errNotConnected = errors.New("not connected")
	errEmptyHost    = errors.New("empty hostname")
	errBadAddr      = errors.New("no IPv4 address")
)

// start brings the radio up, once. A failure is kept and returned by every
// call after it, since espradio cannot be enabled twice.
func (w *wifiLink) start() error {
	w.up.Do(func() {
		w.upErr = espradio.Enable(espradio.Config{Logging: espradio.LogLevelError})
		if w.upErr == nil {
			w.upErr = espradio.Start()
		}
	})
	return w.upErr
}

func (w *wifiLink) scan() ([]application.AP, error) {
	w.radio.Lock()
	defer w.radio.Unlock()
	if err := w.start(); err != nil {
		return nil, err
	}
	found, err := espradio.Scan()
	if err != nil {
		return nil, err
	}
	aps := make([]application.AP, len(found))
	for i, ap := range found {
		aps[i] = application.AP{SSID: ap.SSID, RSSI: ap.RSSI}
	}
	return aps, nil
}

func (w *wifiLink) join(ssid, password string) error {
	w.radio.Lock()
	defer w.radio.Unlock()
	if err := w.start(); err != nil {
		return err
	}

	// Whatever happens next, the old network is gone.
	w.setStatus(application.WiFiStatus{})

	err := espradio.Connect(espradio.STAConfig{SSID: ssid, Password: password})
	if err != nil {
		return err
	}
	stack, err := w.stack()
	if err != nil {
		return err
	}
	if _, err := stack.SetupWithDHCP(espradio.DHCPConfig{}); err != nil {
		return err
	}

	s := application.WiFiStatus{Connected: true, SSID: ssid}
	addr4 := stack.LnetoStack().Addr4()
	if addr, ok := netip.AddrFromSlice(addr4[:]); ok && !addr.IsUnspecified() {
		s.IP = addr.String()
	}
	w.setStatus(s)
	return nil
}

// stack returns the network stack, creating it and starting its goroutine
// on the first join.
func (w *wifiLink) stack() (*espradio.Stack, error) {
	var err error
	w.stackOnce.Do(func() {
		var nd *espradio.NetDev
		nd, err = espradio.StartNetDev()
		if err != nil {
			return
		}
		var s *espradio.Stack
		s, err = espradio.NewStack(nd, espradio.StackConfig{
			Hostname:     hostname,
			MaxUDPPorts:  2,
			MaxTCPPorts:  1,
			PassivePeers: 64,
		})
		if err != nil {
			return
		}
		gostack := s.LnetoStack().StackGo(pollBackoff, xnet.StackGoConfig{
			ListenerPoolConfig: xnet.TCPPoolConfig{
				PoolSize:           4,
				QueueSize:          4,
				TxBufSize:          4096,
				RxBufSize:          1024,
				EstablishedTimeout: 2 * time.Second,
				ClosingTimeout:     2 * time.Second,
				NewBackoff:         func() lneto.BackoffStrategy { return pollBackoff },
			},
		})
		w.mu.Lock()
		w.netstack = s
		w.berkeley = xnet.NewBerkeleyStack(gostack.Socket)
		w.mu.Unlock()
		go pumpStack(s)
	})
	if err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.netstack == nil {
		// An earlier first join failed half way; espradio has no way to
		// try again without a reboot.
		return nil, errNotConnected
	}
	return w.netstack, nil
}

func (w *wifiLink) setStatus(s application.WiFiStatus) {
	w.mu.Lock()
	w.state = s
	w.mu.Unlock()
}

// status returns what the last join left. espradio does not report a
// network that drops later, so neither does this.
func (w *wifiLink) status() application.WiFiStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state
}

// station is the link as the Wi-Fi page sees it. It is a type of its own
// because application.WiFi and netdev.Netdever both have a Connect, one to
// join a network and one to open a socket.
type station struct{ link *wifiLink }

func (s station) Scan() ([]application.AP, error)     { return s.link.scan() }
func (s station) Connect(ssid, password string) error { return s.link.join(ssid, password) }
func (s station) Status() application.WiFiStatus      { return s.link.status() }

// pumpStack moves packets between the radio and the stack, forever.
//
// The scheduler is cooperative, so a busy pass must still yield: on a
// network with steady broadcast traffic there is a frame to handle on every
// pass, and without the yield the loop never gives the UI goroutine a turn,
// which froze the screen from the moment a join succeeded.
func pumpStack(s *espradio.Stack) {
	for {
		send, recv, _ := s.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(pollTime)
		} else {
			runtime.Gosched()
		}
	}
}

// sockets returns the stack and its sockets, or an error before the first
// successful join, so that a request made too early fails instead of
// panicking.
func (w *wifiLink) sockets() (*espradio.Stack, *xnet.StackBerkeley, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.berkeley == nil || !w.state.Connected {
		return nil, nil, errNotConnected
	}
	return w.netstack, w.berkeley, nil
}

// The rest is netdev.Netdever, for net and net/http.

func (w *wifiLink) GetHostByName(name string) (netip.Addr, error) {
	if name == "" {
		return netip.Addr{}, errEmptyHost
	}
	if name[0] >= '0' && name[0] <= '9' {
		return netip.ParseAddr(name)
	}
	s, _, err := w.sockets()
	if err != nil {
		return netip.Addr{}, err
	}
	addrs, err := s.LnetoStack().StackRetrying(pollBackoff).DoLookupIP(name, 5*time.Second, 3)
	if err != nil {
		return netip.Addr{}, err
	}
	return addrs[0], nil
}

func (w *wifiLink) Addr() (netip.Addr, error) {
	s, _, err := w.sockets()
	if err != nil {
		return netip.Addr{}, err
	}
	addr4 := s.LnetoStack().Addr4()
	addr, ok := netip.AddrFromSlice(addr4[:])
	if !ok {
		return netip.Addr{}, errBadAddr
	}
	return addr, nil
}

func (w *wifiLink) Socket(domain, stype, protocol int) (int, error) {
	_, b, err := w.sockets()
	if err != nil {
		return -1, err
	}
	return b.Socket(domain, stype, protocol)
}

func (w *wifiLink) Bind(fd int, ip netip.AddrPort) error {
	_, b, err := w.sockets()
	if err != nil {
		return err
	}
	return b.Bind(fd, ip)
}

func (w *wifiLink) Connect(fd int, host string, ip netip.AddrPort) error {
	_, b, err := w.sockets()
	if err != nil {
		return err
	}
	// net passes the hostname with no address when it wants the netdev to
	// resolve it, as DialTLS does.
	if (!ip.Addr().IsValid() || ip.Addr().IsUnspecified()) && host != "" {
		addr, err := w.GetHostByName(host)
		if err != nil {
			return err
		}
		ip = netip.AddrPortFrom(addr, ip.Port())
	}
	return b.Connect(fd, host, ip)
}

func (w *wifiLink) Listen(fd, backlog int) error {
	_, b, err := w.sockets()
	if err != nil {
		return err
	}
	return b.Listen(fd, backlog)
}

func (w *wifiLink) Accept(fd int) (int, netip.AddrPort, error) {
	_, b, err := w.sockets()
	if err != nil {
		return -1, netip.AddrPort{}, err
	}
	return b.Accept(fd)
}

func (w *wifiLink) Send(fd int, buf []byte, flags int, deadline time.Time) (int, error) {
	_, b, err := w.sockets()
	if err != nil {
		return 0, err
	}
	return b.Send(fd, buf, flags, deadline)
}

func (w *wifiLink) Recv(fd int, buf []byte, flags int, deadline time.Time) (int, error) {
	_, b, err := w.sockets()
	if err != nil {
		return 0, err
	}
	return b.Recv(fd, buf, flags, deadline)
}

func (w *wifiLink) Close(fd int) error {
	_, b, err := w.sockets()
	if err != nil {
		return err
	}
	return b.Close(fd)
}

func (w *wifiLink) SetSockOpt(fd, level, opt int, value interface{}) error {
	_, b, err := w.sockets()
	if err != nil {
		return err
	}
	return b.SetSockOpt(fd, level, opt, value)
}
