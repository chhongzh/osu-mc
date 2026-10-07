# osu!mc

osu!microcontroller 的缩写。路径里不能有 `!`，所以模块名、import 路径和 `cmd/osu-mc` 目录用 `osu-mc`，界面上显示的名字一律写 `osu!mc`。

仿 osu!(lazer) 的 UI 框架，TinyGo 0.42，目标 `esp32s3-generic`，屏幕是 128x64 SH1106 OLED。

- `cmd/osu-mc`：固件入口（引脚常量、按键、屏幕、主循环与熄屏），Wi-Fi（espradio + netdev）和 HTTP 的硬件实现，以及系统信息（`system.go`：eFuse、链接器符号、`runtime.ReadMemStats`、主循环统计）
- `internal/application`：所有应用和菜单（启动器、设置/配网/关于、Lorem Ipsum、HTTP 客户端、系统信息），只依赖 `Services` 接口，可在主机上测试
- `internal/animation`：`animation.Value` 与全局 `animation.Default`
- `internal/ui`：ListItem、List/Row、光标、Viewer、事件路由、键盘、输入框
- `driver`：屏幕驱动

## 核心约束：所有数值都是 animation.Value

屏幕上看得见的每一个数都必须是 `animation.Value`，这包括位置、尺寸、偏移、滚动、拉扯（pull）、光标的倾斜/挤压、页面的进出。只改 `Target`，让弹簧去追，**不要直接写 `Current`**，否则画面会跳变。

- 新的可动数值：用 `Init(v, speed)` 注册到 `animation.Default`，需要回弹就再调 `Spring(speed, damping)`。在拥有者的 `Release()` 里注销，在 `Snap()` 里收尾。
- 布局按 **Target** 来算，不按 Current 算，这样动画进行到一半时布局也是稳定的。例外只有 Text 的 `LiveFocus` 轴。
- 一个效果只能有一个动画源。两个东西需要同步移动时（比如 pull 时的内容和光标），给它们用同样的弹簧参数和同样的目标，不要让一个去追另一个。
- 直接写 Current 只在三种情况下允许：
  - 子元素被父元素"携带"，不允许各自漂移，例如 `Input.place()`；
  - `Snap()`/`Jump()` 本来就是要立刻到位；
  - 设定动画的**起点**，之后交给弹簧追 Target：页面进场的 `trail()`、新插入项从高度 0 展开的 `placeNew()`。
- 现状：`seq`（List/Row 共用）的 `scroll`、`content` 是普通 float32。它们只用来算出子元素的 Target，本身从不直接画出来。新增的状态如果会直接影响绘制，必须是 `animation.Value`。

## 万物都是 ListItem

屏幕太小，所有组件的表现形式都是"列表里的一行"。没有 Column 或其他布局容器：一页就是一个 `List`，里面的每一行都是 `ListItem`（见 `item.go`）。

```go
type ListItem interface {
    Element
    Frame() *AnimatedRect     // 容器设置它的 Target
    Size() (w, h float32)     // 想要的尺寸；List 用 h，Row 用 w
    Selectable() bool         // false 时选择会跳过它
    Snap()
    Release()
}
```

- 现有的 ListItem：`Text`（`Static = true` 时是不可选的标题/标签）、`Input`、`Row`（横向的一行选项）、`List`（嵌套列表）。
- `List` 和 `Row` 共用 `seq`（`item.go`），所以 API 一样：
  - 读：`Len()`、`Item(i)`、`Index(it)`、`Selected()`、`SelectedItem()`；
  - 改：`Add(items...)`、`Insert(i, it)`、`Remove(i)`、`SetItems(items...)`、`Select(i)`、`Move(delta)`；
  - 改内容：直接改 item 本身，例如 `(*Text).SetText`，下一帧布局自然跟上。
- 增删时选择留在原来那一项上，光标跟着；被删的是选中项时，选择和光标移到接替它的一项，删空了光标隐藏（`Focus(nil)`）。
- 已经上屏的列表：插入项从高度/宽度 0 展开，被删项收拢到 0 后才 `Release()`（`leaving`）；还没布局过的列表增删是瞬时的。`SetItems` 立即释放旧项。
- 新组件要进列表，就实现 `ListItem`；需要时再实现这些可选接口：`Seeker`（上下进入时选最近的一项）、`liveFocuser`、`marqueer`、`cascader`、`holder`。
- 容器的 `Focused()` 在没有可选项时必须返回 nil 接口。

## 交互模型

- 事件分为 `Press` / `Repeat` / `Release`。按下给反馈：`step()` 让光标倾斜，`click()` 让光标挤压。**松开才执行**：确认、返回、打开键盘都在松开时发生。
- `Router` 先走捕获阶段（`InterceptEvent`），再走冒泡阶段（`HandleEvent`）。收到 Press 的元素，同时拥有它后续的 Repeat 和 Release。
- 到了边界（List/Row 已经到顶、到底、到头）：按住时内容和光标一起往那个方向偏移 `pullDist`，松开后弹回原位。不弹跳，不 Kick（见 `nav.go` 的 `push`）。
- 左键 = 返回：按下时 Viewer 露出一点上一页（peek），松开时 pop。

## 固件约束

- `internal/ui` 不能 import `machine`，必须能在主机上 `go test`。
- 每帧不分配内存：Update/Draw 里不能 append、拼接字符串，也不能生成闭包。
- 固件里不要用 `strings` 包，它会带进约 10KB 的 Unicode 表。需要时手写 ASCII 版本。
- 带 animation.Value 的元素必须用 `NewXxx` 创建，并且不能复制。

## 验证

```
gofmt -l cmd internal
go vet ./internal/...
go test ./internal/...
tinygo build -target esp32s3-generic -o /tmp/osu-mc.bin ./cmd/osu-mc
```

渲染类测试（`TestRender*`）用 `go test -v` 可以看到 ASCII 画面。Viewer 自己会画光标，测试里要用 `frame(v)`，不能再叠加光标，否则反色画两次会互相抵消。

## 环境

- 不是 git 仓库。
- zsh 开了 noclobber，`>` 覆盖会失败，改用 Write 工具或 python 写文件。
- 不允许删除文件（`rm` 被禁止）。
- 注释写成完整的英文句子，用来解释"为什么"，风格和现有代码一致。
