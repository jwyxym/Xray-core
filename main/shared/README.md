# Xray 动态库

独立的 C ABI 入口，可从 C/C++、Rust、C# 等语言加载。命令行程序仍使用 `./main`，动态库使用 `./main/shared`。

## GitHub Actions 构建

工作流：`.github/workflows/shared-library.yml`，显示名称 **Build Shared Libraries**。

提交、Pull Request、发布 Release 或手动运行工作流均会构建。成功后在该次 Actions 运行的 **Artifacts** 下载：

| Artifact | 平台 | 动态库 |
|---|---|---|
| `xray-shared-windows-amd64` | Windows x64 | `xray.dll` |
| `xray-shared-windows-arm64` | Windows ARM64 | `xray.dll` + ARM64 `wintun.dll` |
| `xray-shared-linux-amd64` | Linux x64 | `libxray.so` |
| `xray-shared-linux-arm64` | Linux ARM64 | `libxray.so` |
| `xray-shared-darwin-arm64` | macOS Apple Silicon | `libxray.dylib` |
| `xray-shared-android-arm64-v8a` | Android ARM64 | `libxray.so` |
| `xray-shared-android-armeabi-v7a` | Android ARMv7 | `libxray.so` |
| `xray-shared-android-x86` | Android x86 | `libxray.so` |
| `xray-shared-android-x86_64` | Android x86-64 | `libxray.so` |

每个包包含动态库、生成的 `xray.h`、C 调用验证示例 `smoke.c`、本文档、许可证和 `runtime-dependencies.txt` 运行库依赖清单。Windows 包还包含 `wintun.dll`、`LICENSE-Wintun.txt`，以及构建产物实际需要的 MinGW 运行库及其 `licenses/` 许可证。桌面平台 CI 先运行 Go 测试，再通过 `CGO_ENABLED=1 go build -buildmode=c-shared` 构建，最后编译并运行 C 示例验证实际 ABI。产物上传到 Actions Artifacts，不自动附加到 Release。

Android 使用 NDK r28b 交叉编译，目标 API 24（Android 7.0），ARMv7 使用 `GOARM=7`。链接时设置 16 KiB 最大页对齐。CI 检查 ELF 目标架构、五个 C 导出符号，并交叉编译链接 C 示例；不在 Linux runner 上运行 Android 二进制，也不宣称已通过设备运行测试。

Android 集成时，将各 ABI 的 `libxray.so` 放入 `app/src/main/jniLibs/<ABI>/`。这是 C ABI 动态库，不包含 Java/Kotlin JNI 包装；Java/Kotlin 调用需要另行提供 JNI 桥接，原生代码使用对应构建产物的 `xray.h`。

Windows ARM64 使用 `windows-11-arm` 与 MSYS2 CLANGARM64，Linux ARM64 使用 `ubuntu-24.04-arm`，macOS ARM64 使用 `macos-14`。这些任务均在对应 ARM64 runner 上构建、运行 Go 测试与 C 调用测试。Windows 根据 `GOARCH` 从官方 Wintun 包中选择 `bin/arm64/wintun.dll` 或 `bin/amd64/wintun.dll`，并检查文件与许可证均存在且 DLL 未被修改。

目前未配置 macOS Intel 构建。Linux 产物使用 runner 的系统 C 库，部署环境需兼容其 libc 版本。

## TUN 运行依赖与部署

已核对官方 [v26.3.27 Release](https://github.com/XTLS/Xray-core/releases/tag/v26.3.27) 的 Windows x64、Linux x64、macOS ARM64、Android ARM64/x64 ZIP：Windows 附带 Wintun 及许可证；其他所查平台没有额外的 TUN `.so` 或 `.dylib`。本项目的 C shared 构建还会检查实际链接依赖，不能仅凭官方可执行文件的附件推断动态库依赖。

| 平台 | TUN 来源 | 打包与宿主要求 |
|---|---|---|
| Windows | Wintun 驱动 | 随包提供匹配架构的官方签名 `wintun.dll` 0.14.1 及许可证；CI 校验下载包 SHA-256，不修改 DLL |
| Linux | 内核 `/dev/net/tun` | 无第三方 TUN SO；由宿主提供 TUN 设备、权限以及兼容的系统运行库 |
| macOS | 系统 utun | 无第三方 TUN dylib；系统库由 macOS 提供 |
| Android | 宿主 `VpnService` 提供的 TUN fd | 无第三方 TUN SO；宿主创建 VPN 接口，并通过 `XRAY_TUN_FD` 传入同进程的文件描述符 |

当前 Wintun Go 依赖使用 `LOAD_LIBRARY_SEARCH_APPLICATION_DIR | LOAD_LIBRARY_SEARCH_SYSTEM32`。部署时应把 `wintun.dll` 放在**宿主可执行文件旁边**；对 Node.js 宿主而言是 `node.exe` 所在目录，仅放在 JS 脚本或 `xray.dll` 所在目录并不保证能找到。不要把任意系统 DLL 复制到包里或覆盖系统目录中的文件。Windows TUN 通常需要宿主以管理员权限运行。

Windows CI 会递归检查导入表，收集来自 MSYS2 编译器目录的运行库与对应许可证。Linux/macOS/Android 记录并校验系统动态库依赖；出现清单之外的非系统依赖时构建失败，以便显式补全打包。不会把 glibc、Android Bionic 或 macOS 系统库当作可移植附件复制进包。

官方 ZIP 还包含 `geoip.dat`、`geosite.dat`。它们是地理规则数据，不是 TUN 驱动或动态库；当前动态库包不附带，需要使用 `geoip:` / `geosite:` 等规则时由宿主另外配置。

## 接口

以构建产物中的 `xray.h` 为准，导出函数为：

```c
int XrayStart(char *config_json);
int XrayStartArgs(char *args_json);
int XrayStop(void);
char *XrayLastError(void);
void XrayFreeString(char *value);
```

- `XrayStart`：接收 UTF-8、NUL 结尾的 JSON 配置对象。读取输入，不修改或保留输入指针。
- `XrayStartArgs`：接收 UTF-8 JSON 字符串数组，每个元素是一个命令行参数。不包含程序名或 `run`，不执行 shell 拆词。
- 启动成功后即返回，代理在后台运行；启动、停止返回 `0` 表示成功，`-1` 表示失败。
- 每份加载的动态库管理一个实例。重复启动报错，不替换已有实例；停止后可以重新启动；未启动时停止也返回成功。
- `XrayLastError`：返回最近一次启停操作的错误文本副本。成功操作会清空错误；查询本身不清空错误。
- 错误字符串必须调用同一动态库的 `XrayFreeString` 释放，不能用宿主的 `free` 释放。`XrayFreeString(NULL)` 可安全调用。
- 接口内部串行执行，但错误状态由调用方共享。多线程调用方应在同一个宿主锁内执行操作与错误读取。

配置中的 `env`、日志等功能会影响宿主进程的全局状态；停止实例不会还原这些全局设置。动态库入口不注册退出信号处理器，也不调用命令行入口的 `os.Exit`。不要在运行时卸载并重新加载 Go 动态库；进程退出前调用 `XrayStop`，让库保持加载到进程结束。

## 使用 --set

`XrayStartArgs` 支持 `-c` / `-config`、`-confdir`、`-format` 和可重复的 `--set`。例如传入：

```json
[
	"--set", "inbounds[0].protocol=socks",
	"--set", "inbounds[0].listen=127.0.0.1",
	"--set", "inbounds[0].port=1080",
	"--set", "outbounds[0].protocol=freedom"
]
```

文件加覆盖参数：

```json
["-c", "config.json", "--set", "inbounds[0].port=1080"]
```

独立数字、布尔值、数组和对象按 JSON 解析；其他文本作为字符串。例如强制字符串可使用 `"--set", "env.EXAMPLE=\"123\""`。同路径后出现的值优先。数组从下标 `0` 开始，可修改已有元素或在当前长度处追加。

显式文件支持 JSON/JSONC、YAML、TOML；Protobuf 文件可单独加载，但不支持 `--set` 覆盖。目录中的文本配置按文件名顺序合并。相对路径以宿主工作目录为基准。不会自动查找默认配置或环境变量指定的配置文件，不读取 `stdin:`。`-test`、`-dump` 等 CLI 控制选项不属于动态库启动接口。

## C 调用示例

```c
#include <stdio.h>
#include "xray.h"

int main(void) {
	char args[] = "[\"--set\",\"outbounds[0].protocol=freedom\"]";
	if (XrayStartArgs(args) != 0) {
		char *error = XrayLastError();
		fprintf(stderr, "%s\n", error);
		XrayFreeString(error);
		return 1;
	}
	/* 此处运行宿主程序逻辑。 */
	return XrayStop() == 0 ? 0 : 1;
}
```

该最小示例只配置直连出站；实际监听代理端口需添加上述 `inbounds` 参数。完整 ABI 验证见 `smoke.c`（源码位置 `main/shared/example/smoke.c`）。
