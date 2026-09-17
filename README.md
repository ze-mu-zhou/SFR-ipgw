# SFR-ipgw

东北大学校园网命令行工具，提供统一认证、校园网登录与设备管理、计费记录查询、JSON 输出和流量快照对比。

项目在 [ze-mu-zhou/SFR-ipgw](https://github.com/ze-mu-zhou/SFR-ipgw) 独立维护，Go 模块为 `github.com/ze-mu-zhou/SFR-ipgw`。Windows 支持隐藏密码输入与系统凭据管理器。命令名称保持 `ipgw`，便于终端使用。

## 构建与使用

从本仓库获取代码；完整凭据管理、计费比较和构建说明见 [Windows 使用说明](WINDOWS_USAGE.md)。

Windows PowerShell：

```powershell
git clone https://github.com/ze-mu-zhou/SFR-ipgw.git
cd SFR-ipgw
.\scripts\build.ps1
.\ipgw.exe version
.\ipgw.exe info -u 学号 -a
```

安装与 `go.mod` 相容的 Go 版本。查询时隐藏输入密码；如需保存默认账号：

```powershell
.\ipgw.exe config account add -u 学号 --default
.\ipgw.exe info -a --json
```

Linux、FreeBSD、macOS 可从本仓库构建，非 Windows 系统使用临时密码输入：

```sh
git clone https://github.com/ze-mu-zhou/SFR-ipgw.git
cd SFR-ipgw
go build -o ipgw ./cmd/ipgw
./ipgw info -u 学号 -a
```

直接 `go build` 使用开发构建标记；Windows 构建脚本及 Makefile 会注入提交、版本和构建时间。

## 常用命令

| 命令 | 用途 |
| --- | --- |
| `ipgw login` | 登录校园网 |
| `ipgw logout` | 登出 |
| `ipgw info -a` | 查询套餐、设备和计费记录 |
| `ipgw info -a --json` | 输出结构化查询结果 |
| `ipgw compare --json before.json after.json` | 离线比较快照 |
| `ipgw test` | 检测网络连接 |
| `ipgw version` | 查看版本和构建信息 |

Windows 在项目目录运行时，将命令前缀写为 `.\ipgw.exe`。更多选项见 `ipgw help` 和 `ipgw help 命令名`。

本地开发构建默认禁止自更新覆盖；更新代码后重新构建即可。正式发行入口为 [本仓库 Releases](https://github.com/ze-mu-zhou/SFR-ipgw/releases)，只有维护者发布兼容资产后才能使用。

反馈或贡献请前往 [Issues](https://github.com/ze-mu-zhou/SFR-ipgw/issues) 和 [Pull requests](https://github.com/ze-mu-zhou/SFR-ipgw/pulls)。

## 许可证

采用 MIT 许可，详见 [LICENSE](LICENSE)。沿用代码的版权及来源说明见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
