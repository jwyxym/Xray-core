#!/usr/bin/env bash
set -euo pipefail

# Invoked by shared-library.yml after building; never copies OS libraries.
platform="$1"
output="$2"
library="$3"
report="$output/runtime-dependencies.txt"
printf 'Platform: %s\nLibrary: %s\n' "$platform" "$library" > "$report"

case "$platform" in
  windows)
    runtime_dir="$(cygpath -u "$RUNNER_TEMP")/xray-shared-runtime"
    mkdir -p "$runtime_dir"
    curl -fL --retry 3 https://www.wintun.net/builds/wintun-0.14.1.zip -o "$runtime_dir/wintun.zip"
    printf '%s  %s\n' '07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51' "$runtime_dir/wintun.zip" | sha256sum -c -
    unzip -q -o "$runtime_dir/wintun.zip" -d "$runtime_dir"
    case "$GOARCH" in
      amd64|arm64) wintun_arch="$GOARCH" ;;
      386) wintun_arch=x86 ;;
      *) echo "Unsupported Wintun architecture: $GOARCH" >&2; exit 1 ;;
    esac
    cp "$runtime_dir/wintun/bin/$wintun_arch/wintun.dll" "$output/wintun.dll"
    cp "$runtime_dir/wintun/LICENSE.txt" "$output/LICENSE-Wintun.txt"
    test -s "$output/wintun.dll"
    test -s "$output/LICENSE-Wintun.txt"
    cmp "$runtime_dir/wintun/bin/$wintun_arch/wintun.dll" "$output/wintun.dll"
    printf '\nBundled: official signed Wintun 0.14.1 (%s), LICENSE-Wintun.txt\n' "$wintun_arch" >> "$report"

    # CGO may import GCC/LLVM/winpthreads DLLs. Copy those and their licenses,
    # recursively, while leaving Windows DLLs and API sets to the host OS.
    compiler_dir="$(dirname "$(command -v "$CC")")"
    objdump_tool="${XRAY_OBJDUMP:-objdump}"
    system_dir="$(cygpath -u "$WINDIR")/System32"
    queue=("$library" wintun.dll)
    declare -A visited=()
    for ((index=0; index<${#queue[@]}; index++)); do
      file="${queue[index]}"
      if [[ -n "${visited[${file,,}]:-}" ]]; then
        continue
      fi
      visited[${file,,}]=1
      imports="$("$objdump_tool" -p "$output/$file")"
      printf '\nImports of %s:\n' "$file" >> "$report"
      while IFS= read -r dependency; do
        [[ -n "$dependency" ]] || continue
        printf '  %s\n' "$dependency" >> "$report"
        if [[ -f "$output/$dependency" ]]; then
          queue+=("$dependency")
        elif [[ -f "$compiler_dir/$dependency" ]]; then
          cp "$compiler_dir/$dependency" "$output/$dependency"
          queue+=("$dependency")
          package="$(pacman -Qoq "$compiler_dir/$dependency")"
          package_files="$(pacman -Ql "$package")"
          license_count=0
          while IFS= read -r license; do
            [[ -f "$license" ]] || continue
            destination="$output/licenses/${license#*/share/licenses/}"
            mkdir -p "$(dirname "$destination")"
            cp "$license" "$destination"
            license_count=$((license_count + 1))
          done < <(printf '%s\n' "$package_files" | awk '$2 ~ /\/share\/licenses\// {print $2}')
          if [[ "$license_count" == 0 ]]; then
            echo "No redistribution license found for $dependency ($package)" >&2
            exit 1
          fi
          printf '  Bundled from MSYS2 package: %s\n' "$package" >> "$report"
        elif [[ -f "$system_dir/$dependency" ]]; then
          : # Supplied by Windows.
        else
          case "${dependency,,}" in
            api-ms-win-*|ext-ms-win-*) : ;;
            *) echo "Unresolved runtime DLL: $dependency (imported by $file)" >&2; exit 1 ;;
          esac
        fi
      done < <(printf '%s\n' "$imports" | sed -n 's/^[[:space:]]*DLL Name: //p' | tr -d '\r')
    done
    ;;

  linux|android)
    readelf_tool="${READELF:-readelf}"
    dynamic_section="$("$readelf_tool" --dynamic "$output/$library")"
    printf '\nELF dynamic section:\n%s\n' "$dynamic_section" >> "$report"
    while IFS= read -r dependency; do
      case "$platform:$dependency" in
        linux:libc.so.6|linux:libm.so.6|linux:libpthread.so.0|linux:libdl.so.2|linux:librt.so.1|linux:libresolv.so.2|linux:libgcc_s.so.1|linux:ld-linux-*.so.*) : ;;
        android:libc.so|android:libm.so|android:libdl.so|android:liblog.so|android:libandroid.so) : ;;
        *) echo "Unbundled non-system dependency: $dependency" >&2; exit 1 ;;
      esac
    done < <(printf '%s\n' "$dynamic_section" | sed -n 's/.*(NEEDED).*\[\([^]]*\)\].*/\1/p')
    if [[ "$platform" == linux ]]; then
      resolved="$(ldd "$output/$library")"
      printf '\nResolved system libraries:\n%s\n' "$resolved" >> "$report"
      if [[ "$resolved" == *'not found'* ]]; then
        echo 'Unresolved Linux runtime library' >&2
        exit 1
      fi
    fi
    printf '\nTUN uses OS facilities; no third-party TUN shared library is bundled.\n' >> "$report"
    ;;

  darwin)
    dependencies="$(otool -L "$output/$library")"
    printf '\nMach-O dependencies:\n%s\n' "$dependencies" >> "$report"
    # First entry is the dylib's own install name, not a dependency.
    while IFS= read -r dependency; do
      case "$dependency" in
        /usr/lib/*|/System/Library/*) : ;;
        *) echo "Unbundled non-system dependency: $dependency" >&2; exit 1 ;;
      esac
    done < <(printf '%s\n' "$dependencies" | tail -n +3 | awk '{print $1}')
    printf '\nTUN uses macOS utun; no third-party TUN dylib is bundled.\n' >> "$report"
    ;;

  *) echo "Unsupported platform: $platform" >&2; exit 1 ;;
esac

cat "$report"
