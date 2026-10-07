package main

import (
	"io"
	"net/http"

	"osu-mc/internal/application"
)

// maxBody is how much of a response body is kept. The screen shows a few
// hundred characters at a time, and the heap is small: net/http already
// takes about 10kB per request.
const maxBody = 4096

// httpClient makes requests with net/http, through the netdev that main
// installs, so it works once Wi-Fi is connected.
type httpClient struct{}

func (httpClient) Get(url string) (application.Response, error) {
	resp, err := http.Get(url)
	if err != nil {
		return application.Response{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return application.Response{}, err
	}
	return application.Response{Status: resp.Status, Body: string(body)}, nil
}
