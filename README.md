# 国际新闻滚动条

Windows 原生小窗新闻条，无品牌标题栏，新闻文字为 12 pt。窗口默认贴在工作区底部、任务栏上方；在新闻区域拖动可移动窗口，拖动边框或角落可调大小。

右侧按钮依次为“暂停/继续”“贴底”和“关闭”。每条标题前都会标注地区与新闻媒体。新闻每 5 分钟更新；网络断开时显示状态，不使用模拟新闻。

新闻源包括美国 NPR、欧洲 The Guardian 和 France 24、香港 RTHK 和 SCMP、新加坡 CNA、日本 NHK WORLD 和 Japan Times。能否访问取决于当前网络。

英文标题会通过 MyMemory 翻译为简体中文，成功结果会缓存于程序运行期间。翻译服务不可用或额度耗尽时，该条会标注“原文”并显示英文标题。翻译服务会收到公开新闻标题。

## 构建和启动

需要 Go 1.23 或更新版本，不依赖第三方 Go 包：

```powershell
go test ./...
go build -ldflags="-H windowsgui" -o ticker.exe ticker.go
```

也可双击 `build.bat` 构建，之后双击 `start-ticker.bat` 启动。

## 配置

在 `config.json` 中可设置字号、新闻刷新间隔和滚动速度。窗口位置和大小可直接用鼠标调整。
