# 谢尔顿 Wi-Fi 警告 / Sheldon Wi-Fi Warning

基于 [Shizzi](https://github.com/carlelieser/shizzi) 改造的 Android 热点警告实验项目：希望访客连接手机热点后，通过系统的网络登录提示进入警告页，播放录音后继续上网。

**当前状态：暂停开发的原型。代码已构建并安装过测试机，但没有完成真实访客设备的自动弹页、恢复上网及重连验收。请勿当作已验证可用的成品。**

## 采用的方案

手机运行 Shizuku 和本项目 App。借助 Shizzi 的测试 TUN 网络接管原生热点的上游流量，网页、录音与转发逻辑均在手机本地运行。运行期间不需要 Mac 提供网页或转发流量。

使用共享警告状态，以适应 Android 热点在流量进入 TUN 前执行 IPv4 NAT 的情况：

1. 开始共享后进入待警告状态。
2. HTTP 请求重定向到本地警告页；待放行期间拒绝新的 HTTPS 连接和非 DNS UDP 流量，以触发访客系统的认证门户检测。
3. 访客点击播放录音；播放结束后提交共享放行，也可点击“继续上网”。
4. 一人完成后整组恢复上网。
5. 检测到新 Wi-Fi 客户端关联后重新启用共享警告。

不做逐设备白名单或独立放行。新访客加入后，已连接访客的新请求也可能受到影响。已有长连接不保证立即断开。

## 已实现与验证范围

| 内容 | 状态 |
|---|---|
| 手机本地中文警告页、原始录音、手动播放 | 已实现 |
| HTTP 引导、共享放行、重新启用警告 | 已实现，并通过主机端 TCP 数据流测试 |
| 音频分段加载（Range）、旧放行凭据失效 | 已通过测试 |
| 警告开关、手动再次警告、本机预览 | 已实现 |
| Wi-Fi 关联回调及轮询后备 | 已实现，未完成真实重连验收 |
| Android APK 构建及 USB 安装 | 已在 Redmi Turbo 3 / Android 16 上完成 |
| 多种访客系统的实际自动弹页及放行 | **未完成验收** |
| 手机内导入自定义录音/图片 | 未实现；目前需修改素材并重新构建 |
| iPhone 个人热点作为门户主机 | 无公开 API 实现方案；未开发 iOS 版本 |

自动弹页由访客系统决定，可能表现为“登录 Wi-Fi”通知，不能保证所有系统每次重连都自动打开窗口。严格私有 DNS、VPN 和缓存的联网检测结果也可能影响行为。不会伪造 HTTPS 证书，不承诺有声自动播放。

## 运行要求

- Android 11+、ARM；具体兼容性取决于系统的 Tethering/TestNetwork 接口，Android 13+ 更适合验证。
- [Shizuku](https://github.com/RikkaApps/Shizuku) 13.6.0+ 已启动且对 App 授权。
- 原生热点能正常共享网络。
- 初次构建及安装可使用 Mac；之后网页和录音留在手机内。

本项目没有替换系统模块。若兼容性检查失败，不应为了本项目直接安装或替换 Tethering APEX。

## 快速开始

1. 从源码构建 APK，并安装到测试手机。
2. 启动 Shizuku，打开“谢尔顿热点”，完成授权与兼容性引导。
3. 在主页开始共享，让测试设备连接手机原生热点。
4. 观察测试设备是否出现网络登录提示，播放录音或选择继续上网。
5. 在 App 设置 → “谢尔顿警告设置与预览”中关闭警告、再次启用或预览页面。

共享运行时，也可以手动打开 `http://198.18.0.1/`。手动打开成功不代表自动弹页成功。

关闭“启用热点警告”开关可以保留共享；主页停止共享会停止本轮热点会话。手机重启后通常需要重新启动 Shizuku。

详细说明见 [使用说明](使用说明-谢尔顿热点.md)，当前开发状态见 [STATUS](docs/sheldon/STATUS.md)。

## 构建与测试

依赖 JDK 17、Android SDK 35、NDK `27.0.12077973`、Go `1.26.3` 及 gomobile。SDK 路径可通过 `ANDROID_HOME` 或 `local.properties` 指定。

```sh
export JAVA_HOME=/path/to/jdk17
export ANDROID_HOME=/path/to/android-sdk
export GOTOOLCHAIN=go1.26.3
export PATH="$JAVA_HOME/bin:$HOME/go/bin:$PATH"
go install golang.org/x/mobile/cmd/gomobile@v0.0.0-20260803200217-62cee1672c8e
gomobile init
./gradlew assembleDebug --no-daemon
```

Mac / Homebrew 用户也可设置对应环境变量后运行 `./build-sheldon.command`。

输出：`app/build/outputs/apk/debug/app-debug.apk`。应用标识为 `dev.shizzi.sheldon`，与官方 Shizzi 区分。

```sh
cd datapath
GOTOOLCHAIN=go1.26.3 go test -race ./...
```

测试覆盖真实 netstack TCP 数据流的“检测请求 → 警告页 → 放行 → 上游请求 → 再次警告”，但不替代手机实测。

## 源码导航

- `datapath/warning.go`：共享警告状态、HTTP 引导、页面与音频服务。
- `datapath/warning_assets/`：提供给访客的页面、文案与录音。
- `app/src/main/java/dev/shizzi/WarningConnections.kt`：关联事件及轮询后备。
- `WarningSettingsActivity.kt` / `WarningPreviewActivity.kt`：手机设置与本机预览。
- `tools/CapabilityProbe.java`：诊断系统接口；默认只读，不切换热点上游。

## 录音与来源

原始警告录音按仓库发布者要求公开，位于 [warning.m4a](datapath/warning_assets/warning.m4a)，预览素材目录内有一份相同副本。文件属于用户提供素材，不应将代码许可证视为对录音的人声、人格或第三方权利的额外授权。

代码来源为 Shizzi v0.6.0，基线提交 `c22f24f577755fb5dd0414d2b4adc3199d5ef84d`。保留 [MIT 许可证](LICENSE)、[原项目 README](README.upstream.md) 和原始历史更新说明。此仓库为独立实验衍生项目，不是上游官方版本。
