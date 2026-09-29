#!/usr/bin/env bash
# edited from deno_install
#
# 默认安装到 ~/.local/bin，无需 root 权限。
# 安装到系统目录：sudo IPGW_INSTALL_DIR=/usr/local/bin bash install.sh
set -euo pipefail

fail() {
	echo "Error: $*" 1>&2
	exit 1
}

for tool in curl unzip; do
	command -v "$tool" >/dev/null || fail "$tool is required to install ipgw."
done

if command -v sha256sum >/dev/null; then
	sha256() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null; then
	sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
else
	fail "sha256sum or shasum is required to install ipgw."
fi

# 与 Makefile 的 PLATFORM_LIST 保持一致。
supported="darwin-amd64 darwin-arm64 linux-386 linux-amd64 linux-arm linux-arm64 linux-mips64 linux-mips64le freebsd-386 freebsd-amd64 windows-386 windows-amd64 windows-arm64"

little_endian() {
	[ "$(printf '\001\000' | od -An -tx2 | tr -d ' \n')" = "0001" ]
}

detect_target() {
	local os arch machine
	if [ "${OS:-}" = "Windows_NT" ]; then
		os="windows"
		machine="${PROCESSOR_ARCHITEW6432:-${PROCESSOR_ARCHITECTURE:-}}"
	else
		case "$(uname -s)" in
		Darwin) os="darwin" ;;
		Linux) os="linux" ;;
		FreeBSD) os="freebsd" ;;
		*) fail "unsupported operating system: $(uname -s)" ;;
		esac
		machine="$(uname -m)"
	fi

	case "$machine" in
	x86_64 | amd64 | AMD64) arch="amd64" ;;
	aarch64 | arm64 | ARM64) arch="arm64" ;;
	armv7* | armv8l) arch="arm" ;;
	i386 | i486 | i586 | i686 | x86 | X86) arch="386" ;;
	mips64)
		if little_endian; then arch="mips64le"; else arch="mips64"; fi
		;;
	mips64el | mips64le) arch="mips64le" ;;
	armv5* | armv6*) fail "$machine is not supported: prebuilt ARM binaries require ARMv7 or newer; please build from source." ;;
	*) fail "unsupported CPU architecture: $machine" ;;
	esac

	# macOS 在 Rosetta 下运行 shell 时 uname -m 会报告 x86_64，优先使用原生 arm64。
	if [ "$os" = "darwin" ] && [ "$arch" = "amd64" ] &&
		[ "$(sysctl -in hw.optional.arm64 2>/dev/null || true)" = "1" ]; then
		arch="arm64"
	fi

	echo "$os-$arch"
}

target="$(detect_target)"
case " $supported " in
*" $target "*) ;;
*) fail "no prebuilt ipgw release for $target; please build from source." ;;
esac

executable="ipgw"
case "$target" in
windows-*) executable="ipgw.exe" ;;
esac

bin_dir="${IPGW_INSTALL_DIR:-$HOME/.local/bin}"
target_path="$bin_dir/$executable"

if [ ! -d "$bin_dir" ]; then
	mkdir -p "$bin_dir" 2>/dev/null ||
		fail "cannot create $bin_dir. Choose another directory with IPGW_INSTALL_DIR, or run with sudo for a system directory."
fi
if [ ! -w "$bin_dir" ] || { [ -e "$target_path" ] && [ ! -w "$target_path" ]; }; then
	fail "no write permission for $bin_dir. Run with sudo (e.g. sudo IPGW_INSTALL_DIR=$bin_dir bash install.sh), or choose a writable directory with IPGW_INSTALL_DIR."
fi

release_url="https://github.com/ze-mu-zhou/SFR-ipgw/releases/latest/download"
archive="ipgw-${target}.zip"

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/ipgw-install.XXXXXX")"
trap 'rm -rf -- "$tmp_dir"' EXIT

echo "Downloading $archive..."
curl --fail --location --progress-bar --output "$tmp_dir/$archive" "$release_url/$archive"
curl --fail --silent --show-error --location --output "$tmp_dir/checksums.txt" "$release_url/checksums.txt"

expected=$(awk -v f="$archive" '{name=$2; sub(/^\*/, "", name); if (name == f) print $1}' "$tmp_dir/checksums.txt")
actual=$(sha256 "$tmp_dir/$archive")
if [ -z "$expected" ] || [ "$actual" != "$expected" ]; then
	fail "checksum verification failed for $archive"
fi

unzip -q -o "$tmp_dir/$archive" "$executable" -d "$tmp_dir/extract"
[ -f "$tmp_dir/extract/$executable" ] || fail "$archive does not contain $executable"
chmod 0755 "$tmp_dir/extract/$executable"
# 先复制到同目录临时文件再 rename，避免覆盖正在运行的旧版本时出现半写入文件。
cp "$tmp_dir/extract/$executable" "$target_path.tmp.$$"
mv -f "$target_path.tmp.$$" "$target_path"

echo "ipgw was installed successfully to $target_path"
case ":$PATH:" in
*":$bin_dir:"*)
	echo "Run 'ipgw --help' to get started"
	;;
*)
	case "$(basename -- "${SHELL:-}")" in
	zsh) shell_profile=".zshrc" ;;
	bash)
		if [ "$(uname -s)" = "Darwin" ]; then shell_profile=".bash_profile"; else shell_profile=".bashrc"; fi
		;;
	fish) shell_profile=".config/fish/config.fish" ;;
	*) shell_profile=".profile" ;;
	esac
	echo "$bin_dir is not in your PATH. Add it to \$HOME/$shell_profile (or similar):"
	if [ "$shell_profile" = ".config/fish/config.fish" ]; then
		echo "  fish_add_path \"$bin_dir\""
	else
		echo "  export PATH=\"$bin_dir:\$PATH\""
	fi
	echo "Run '$target_path --help' to get started"
	;;
esac
