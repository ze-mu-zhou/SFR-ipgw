# Windows 使用说明

[SFR-ipgw](https://github.com/ze-mu-zhou/SFR-ipgw) 的 Windows 构建、账号管理和计费查询说明。请从本仓库克隆并构建，命令名称为 `ipgw`。

在项目目录使用 PowerShell，安装与 `go.mod` 相容的 Go 版本后构建：

```powershell
.\scripts\build.ps1
.\ipgw.exe version
```

脚本写入版本、提交号、UTC 构建时间和本地修改标记，不提交代码。本地构建的 `update` 会明确拒绝覆盖，避免丢失认证修复。

## 凭据

一次性查询使用隐藏密码输入：

```powershell
.\ipgw.exe info -u 学号 -a
```

保存到 Windows 凭据管理器并设为默认账号：

```powershell
.\ipgw.exe config account add -u 学号 --default
```

配置文件只保留账号和凭据引用，密码保存在 Windows 凭据管理器。非 Windows 系统不支持这里的凭据存储，可使用临时输入。

删除账号（`config account del`）会同时删除凭据管理器中的对应条目；`config account set` 修改密码后，旧凭据条目也会自动清理。

旧凭据仅在配置文件成功保存后清理。保存失败时保留原配置和旧凭据，并尝试回收本次新建的凭据；保存成功但旧凭据清理失败时，命令仍成功，并在标准错误输出中显示警告。

## 查询与计费比较

```powershell
.\ipgw.exe info -a
.\ipgw.exe info -a --json
.\ipgw.exe info -i --snapshot before.snapshot.json --period 2026-09
# 完成需要观察的联网操作后再次查询
.\ipgw.exe info -i --snapshot after.snapshot.json --period 2026-09
.\ipgw.exe compare before.snapshot.json after.snapshot.json
.\ipgw.exe compare --json before.snapshot.json after.snapshot.json
```

`--period` 是用户声明的计费周期，不代表服务器确认。比较要求同一账号、同一已知周期、成功查询且时间有序。单位无法明确换算时，在两次 `info --snapshot` 命令中依据计费系统实际规则显式指定 `--traffic-base 1000` 或 `--traffic-base 1024`，不要猜测。`compare` 离线读取快照，不重新解释单位或周期。快照包含账号和计费数据，请按个人数据保管。

查询失败会报告具体项目和原因，并返回非零退出码；`info -a` 仍保留其他成功项目。JSON 包含查询状态，不能把失败当作零流量。页面显示有舍入精度、计费也可能延迟，差值为零不能单独证明免流。

## 正式发行构建与自更新

仅当维护者已准备相容的发行仓库，并位于无修改的稳定语义版本标签时，才使用：

```powershell
.\scripts\build.ps1 -Release -ReleaseRepo ze-mu-zhou/SFR-ipgw -SourceRepo ze-mu-zhou/SFR-ipgw
```

发行资产名称为 `ipgw-windows-amd64.zip`（其他平台使用对应 GOOS/GOARCH），压缩包根目录包含 `ipgw.exe`。更新器选择固定版本的资产，要求 GitHub API 提供 SHA-256 摘要，校验下载内容及 Go 可执行文件的平台元数据后才替换。缺少摘要的旧发行包需手动更新。

替换失败会恢复原程序；若恢复也失败，错误中提供备份路径。成功后保留旧程序备份并显示路径，退出更新进程后可自行删除备份。更新不会执行下载包中的程序或脚本。
