# osu!mc

osu!microcontroller 的缩写：一个界面仿 osu!(lazer) 的单片机小项目。路径和模块名用 `osu-mc`（`!` 不能出现在路径里），界面上显示的名字一律是 `osu!mc`。

硬件是 ESP32-S3 加一块 128x64 的 SH1106 OLED，用 TinyGo 0.42 构建。屏幕小，UI 不堆布局容器，一切组件都表现为"列表里的一行"，用一个光标上下移动；按键松开才执行确认和返回。

## 功能

启动器里有四个应用，都是可操作的完整页面：

- **Settings**：Wi-Fi 扫描与连接（密码用屏幕上的键盘输入）、关屏、About。
- **Lorem Ipsum**：模拟 osu! 的歌曲选择：搜索、歌曲、难度、mods 和倍速三层页面，只用来展示 UI，不做任何事。
- **HTTP Client**：输入 URL 发起 GET 请求，可保存快捷方式。快捷方式只存在内存里，重启后消失；默认带一条 `http://httpbin.org/get`。
- **System Info**：芯片信息、内存使用、固件在 flash/RAM 里的布局、主循环每帧的负载和网络状态。

## 交互

六个按键加一个复位键，引脚见下方硬件表。

| 键 | 作用 |
| --- | --- |
| UP / DOWN | 移动光标 |
| LEFT / RIGHT | 行内移动，按住重复并逐渐加速；在行首按 LEFT 返回上一页 |
| MID | 确认 |
| SET | 返回 |
| RST | 复位 |

- 事件分 `Press` / `Repeat` / `Release`：按下只给光标倾斜或挤压的反馈，确认、返回、打开键盘都在**松开**时执行。
- 列表到顶或到底时，按住会把内容和光标一起往那个方向推一段距离，松开弹回，不越界、不弹跳。
- 左键返回时 Viewer 先露出一小截上一页（peek），松开才 pop。

## 界面模型

一页就是一个 `List`，里面的每一行都是 `ListItem`（`Text`、`Input`、`Row` 或嵌套的 `List`）。新组件想进列表就实现 `ListItem`。

- 屏幕上看得见的每个数值都是 `animation.Value`（位置、尺寸、滚动、光标倾斜等），只改 `Target` 让弹簧动画去追，不直接写 `Current`。
- 布局按 `Target` 算，动画进行到一半时布局也是稳定的。
- 列表增删时，新项从高度 0 展开，被删项收拢到 0 后才释放。

## 项目结构

```
cmd/osu-mc               固件入口：引脚、按键、屏幕、主循环与熄屏，Wi-Fi 和 HTTP 的硬件实现
internal/application     所有应用和菜单，只依赖 Services 接口，可在主机上测试
internal/ui              列表、光标、Viewer、事件路由、键盘、输入框
internal/animation       动画 Value 和全局动画
driver/sh1106            屏幕驱动
```

`internal/ui` 不 import `machine`，能在主机上 `go test`。动画规则、事件路由、固件约束这些更细的说明写在 [CLAUDE.md](CLAUDE.md)。

## 硬件

屏幕是 10 MHz SPI 的 SH1106；按键用 GPIO 输入下拉，按下读到高电平。引脚避开了 USB、strapping、flash/PSRAM 和 UART0 占用的引脚。

| 功能 | 引脚 |
| --- | --- |
| 屏幕 SCK | GPIO4 |
| 屏幕 SDO | GPIO5 |
| 屏幕 RES | GPIO6 |
| 屏幕 DC | GPIO7 |
| 屏幕 CS | GPIO15 |
| UP / DOWN / LEFT / RIGHT | GPIO16 / 17 / 18 / 8 |
| MID / SET | GPIO3 / 46 |
| RST | GPIO9 |

## 构建与验证

```sh
gofmt -l cmd internal
go vet ./internal/...
go test ./internal/...
tinygo build -target esp32s3-generic -o /tmp/osu-mc.bin ./cmd/osu-mc
```

渲染类测试（`TestRender*`）用 `go test -v` 会输出 ASCII 画面，可以直接看到界面长什么样。
