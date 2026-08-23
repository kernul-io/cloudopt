#!/usr/bin/env bash

set -e

echoexit() {
    local msg="$1"
    local code="$2"
    echo "$msg" >&2
    exit "$code"
}

log() {
    echo "$1" >&2
}

requirements() {
    command -v tar > /dev/null || echoexit 'No `tar` found locally. Please install it using your package manager' 1
    (command -v curl > /dev/null || command -v wget > /dev/null) || \
        echoexit 'No `curl` or `wget` found locally. Please, install it using your package manager'
}

downloader() {
    downloader=
    command -v curl > /dev/null && downloader='curl'
    [[ -z "$downloader" ]] && command -v wget > /dev/null && downloader="wget"
    [[ -z "$downloader" ]] && echoexit 'No `curl` or `wget` found locally. Please, install it using your package manager' 1
    echo "$downloader"
}

binary_name() {
    local bin=
    case "$OSTYPE" in
        linux-gnu*) bin="${bin}linux" ;;
        darwin*) bin="${bin}darwin" ;;
        msys* | cygwin*) bin="${bin}windows" ;;
        # freebsd*) bin="${bin}_freebsd" ;;
        *) echoexit "Unknown operating system: $OSTYPE" 126 ;;
    esac

    if [[ "$OSTYPE" =~ 'darwin' ]]; then
        echo $bin
        return
    fi

    case "$(uname -m)" in
        x86_64) echo "${bin}_amd64" ;;
        aarch64|arm64) echo "${bin}_arm64" ;;
        # armv7l) echo "${bin}_arm32" ;;
        # i686|i386) echo "${bin}_amd32" ;;
        # riscv64) echo "${bin}_riscv64" ;;
        *) echoexit "Unknown architecture: $(uname -m)" 126;;
    esac
}

curl_download() {
    local bin="$1"
    local data=$(curl -s https://api.github.com/repos/kernul-io/cloudopt/releases/latest)
    local tag=$(jq -r '.tag_name' <<< "$data")
    log "Latest release: $tag"
    grep "browser_download_url.*${bin}\.tar\.gz" <<< "$data" \
        | cut -d : -f 2,3 \
        | tr -d \" \
        | xargs curl -sLo /tmp/cloudopt.tar.gz

    grep "browser_download_url.*${bin}_checksums\.txt" <<< "$data" \
        | cut -d : -f 2,3 \
        | tr -d \" \
        | xargs curl -sLo /tmp/cloudopt_checksum.txt

    shasum -cqs -a256 <<< "$(cat /tmp/cloudopt_checksum.txt | cut -d' ' -f1)  /tmp/cloudopt.tar.gz" \
        || echoexit "Checksum invalid. Try to install again later" 1
}

wget_download() {
    local bin="$1"
    local data=$(wget -qO - https://api.github.com/repos/kernul-io/cloudopt/releases/latest)
    local tag=$(jq -r '.tag_name' <<< "$data")
    log "Latest release: $tag"
    grep "browser_download_url.*${bin}\.tar\.gz" <<< "$data" \
        | cut -d : -f 2,3 \
        | tr -d \" \
        | wget -qi - -O /tmp/cloudopt.tar.gz

    grep "browser_download_url.*${bin}_checksums\.txt" <<< "$data" \
        | cut -d : -f 2,3 \
        | tr -d \" \
        | wget -qi - -O /tmp/cloudopt_checksum.txt

    shasum -cqs -a256 <<< "$(cat /tmp/cloudopt_checksum.txt | cut -d' ' -f1)  /tmp/cloudopt.tar.gz" \
        || echoexit "Checksum invalid. Try to install again later" 1
}

print_adjust_path() {
    log
    log
    log 'To make CloudOpt accessible set'
    log '      export PATH="${HOME}/.local/bin:${PATH}"'
    log 'to your init script (~/.bashrc or ~/.config/fish/config.fish)'
    log
    log
}

main() {
    requirements
    local d=$(downloader)
    log "Using $d to download CloudOpt"
    local bname=$(binary_name)
    log "Binary format: $bname"
    ${d}_download "$bname"

    [[ ! -d ~/.local/bin ]] && mkdir -p ~/.local/bin
    tar -xf /tmp/cloudopt.tar.gz -C ~/.local/bin/
    chmod +x ~/.local/bin/cloudopt*

    log 'CloudOpt installed successfuly to ~/.local/bin/cloudopt'
    grep "${HOME}/.local/bin" <<< "$PATH" > /dev/null || print_adjust_path
    export PATH="${HOME}/.local/bin:${PATH}"
    log 'Try to run `cloudopt --help`'
}

main "$@"
