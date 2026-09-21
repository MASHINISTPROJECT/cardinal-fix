#!/bin/sh
set -e

BOLD=$(tput bold 2>/dev/null || echo "")
RED=$(tput setaf 1 2>/dev/null || echo "")
GREEN=$(tput setaf 2 2>/dev/null || echo "")
RESET=$(tput sgr0 2>/dev/null || echo "")

info()  { echo "${BOLD}${GREEN}[cardinal]${RESET} $*"; }
warn()  { echo "${BOLD}${RED}[cardinal]${RESET} $*" >&2; }

FORCE=false
for arg do
    case "$arg" in
        -f|--force|-y|--yes) FORCE=true ;;
    esac
done

info "Uninstalling cardinal..."

# ------------------------------------------------------------------
# 1. Stop and disable systemd units first, so the running supervisor
#    doesn't keep the (soon to be removed) binary pinned.
# ------------------------------------------------------------------
if command -v systemctl >/dev/null 2>&1; then
    for unit in $(systemctl list-units --type=service --all --no-legend 2>/dev/null \
                  | awk '$1 ~ /^cardinal/ {print $1}'); do
        systemctl stop "$unit" 2>/dev/null || true
        systemctl disable "$unit" 2>/dev/null || true
        info "  Stopped $unit"
    done
    for f in /etc/systemd/system/cardinal*.service; do
        [ -e "$f" ] || continue
        rm -f "$f"
        info "  Removed $f"
    done
    systemctl daemon-reload 2>/dev/null || true
fi

# ------------------------------------------------------------------
# 2. Unmount any overlay mounts *before* we blow away the data dir.
# ------------------------------------------------------------------
unmount_overlay() {
    OVERLAY_DIR="${CARDINAL_DIR:-$HOME/.cardinal}/overlay"
    [ -d "$OVERLAY_DIR" ] || return 0
    for d in "$OVERLAY_DIR"/*/merged; do
        [ -d "$d" ] || continue
        if mountpoint -q "$d" 2>/dev/null; then
            umount "$d" 2>/dev/null || umount -l "$d" 2>/dev/null || true
            info "  Unmounted $d"
        fi
    done
}

# ------------------------------------------------------------------
# 3. Remove the binary.
# ------------------------------------------------------------------
PREFIX="${PREFIX:-/usr/local}"
BIN="$PREFIX/bin/cardinal"

if [ -f "$BIN" ]; then
    rm -f "$BIN"
    info "Removed $BIN"
else
    warn "cardinal binary not found at $BIN"
fi

# ------------------------------------------------------------------
# 4. Remove data directory. Honor CARDINAL_DIR, fall back to the
#    conventional system path, then to $HOME/.cardinal.
# ------------------------------------------------------------------
if [ -z "${CARDINAL_DIR:-}" ]; then
    if [ -d /var/lib/cardinal ]; then
        CARDINAL_DIR=/var/lib/cardinal
    else
        CARDINAL_DIR="$HOME/.cardinal"
    fi
fi

if [ -d "$CARDINAL_DIR" ]; then
    echo ""
    if [ "$FORCE" = "true" ]; then
        unmount_overlay
        rm -rf "$CARDINAL_DIR"
        info "Removed $CARDINAL_DIR"
    elif [ ! -t 0 ]; then
        warn "Non-interactive shell — refusing to delete $CARDINAL_DIR."
        warn "Re-run with -f to delete, or remove manually: rm -rf $CARDINAL_DIR"
    else
        warn "WARNING: This will DELETE all images, containers, and data in $CARDINAL_DIR"
        printf "Remove %s? [y/N] " "$CARDINAL_DIR"
        if [ -r /dev/tty ]; then
            read -r confirm </dev/tty
        else
            read -r confirm
        fi
        case "$confirm" in
            y|Y)
                unmount_overlay
                rm -rf "$CARDINAL_DIR"
                info "Removed $CARDINAL_DIR"
                ;;
            *)
                info "Skipped $CARDINAL_DIR (remove manually: rm -rf $CARDINAL_DIR)"
                ;;
        esac
    fi
fi

info "cardinal uninstalled."