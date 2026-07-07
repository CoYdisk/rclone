#!/usr/bin/env python3
"""
rclone cross-compile packaging script.

Cross-compile rclone for specified platforms and output to a configurable directory.

Usage:
    # Default targets (linux/amd64, linux/arm64, windows/386, darwin/amd64)
    python build-rclone.py

    # Custom targets and output directory
    python build-rclone.py --targets linux/amd64 windows/amd64 --out-dir dist

    # Keep debug symbols
    python build-rclone.py --no-strip

    # Specify version and build tags
    python build-rclone.py --version v1.75.0 --tags "cmount"

    # List all available targets
    python build-rclone.py --list-targets
"""

import argparse
import os
import subprocess
import sys

# All supported os/arch pairs (from bin/cross-compile.go)
ALL_TARGETS = [
    "windows/386",
    "windows/amd64",
    "windows/arm64",
    "darwin/amd64",
    "darwin/arm64",
    "linux/386",
    "linux/amd64",
    "linux/arm",
    "linux/arm-v6",
    "linux/arm-v7",
    "linux/arm64",
    "linux/mips",
    "linux/mipsle",
    "freebsd/386",
    "freebsd/amd64",
    "freebsd/arm",
    "freebsd/arm-v6",
    "freebsd/arm-v7",
    "netbsd/386",
    "netbsd/amd64",
    "netbsd/arm",
    "netbsd/arm-v6",
    "netbsd/arm-v7",
    "openbsd/386",
    "openbsd/amd64",
    "plan9/386",
    "plan9/amd64",
    "solaris/amd64",
    "aix/ppc64",
]

# Special environment flags for a given arch
ARCH_FLAGS = {
    "386":    {"GO386": "softfloat"},
    "mips":   {"GOMIPS": "softfloat"},
    "mipsle": {"GOMIPS": "softfloat"},
    "arm":    {"GOARM": "5"},
    "arm-v6": {"GOARM": "6"},
    "arm-v7": {"GOARM": "7"},
}


def get_version(root_dir):
    """Read version from VERSION file."""
    ver_file = os.path.join(root_dir, "VERSION")
    try:
        with open(ver_file, "r") as f:
            return f.read().strip()
    except FileNotFoundError:
        return "unknown"


def get_output_name(goos, goarch):
    """Generate output binary name for the target."""
    base = "rclone-{}-{}".format(goos, goarch)
    if goos == "windows":
        base += ".exe"
    return base


def get_base_arch(goarch):
    """Extract base arch from variant like 'arm-v6' -> 'arm'."""
    return goarch.split("-")[0]


def get_arch_flags(goarch):
    """Get extra environment variables for the given arch."""
    if goarch in ARCH_FLAGS:
        return ARCH_FLAGS[goarch].copy()
    base = get_base_arch(goarch)
    if base in ARCH_FLAGS:
        return ARCH_FLAGS[base].copy()
    return {}


def build_target(goos, goarch, out_dir, ldflags, tags, verbose, index=1, total=1):
    """Build rclone for a single os/arch target. Returns True on success."""
    out_name = get_output_name(goos, goarch)
    out_path = os.path.join(out_dir, out_name)

    sys.stdout.write("[{}/{}] Building {}/{} -> {} ... ".format(
        index, total, goos, goarch, out_name))
    sys.stdout.flush()

    # Build environment
    env = os.environ.copy()
    env["GOOS"] = goos
    env["GOARCH"] = goarch
    for k, v in get_arch_flags(goarch).items():
        env[k] = v

    # Build command
    cmd = ["go", "build"]
    if ldflags:
        cmd += ["-ldflags={}".format(ldflags)]
    if tags:
        cmd += ["-tags", tags]
    cmd += ["-o", out_path, "."]

    if verbose:
        print()
        print("  cmd: {}".format(" ".join(cmd)))
        print("  env: GOOS={} GOARCH={} {}".format(
            goos, goarch,
            " ".join("{}={}".format(k, v) for k, v in get_arch_flags(goarch).items())
        ))

    try:
        proc = subprocess.run(
            cmd,
            cwd=root_dir,
            env=env,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            universal_newlines=True,
        )
        if proc.returncode == 0:
            size = os.path.getsize(out_path)
            size_mb = size / (1024 * 1024)
            print("OK  ({:.1f} MB)".format(size_mb))
            if verbose and proc.stdout.strip():
                print(proc.stdout)
            return True
        else:
            print("FAILED (exit code: {})".format(proc.returncode))
            if proc.stdout.strip():
                for line in proc.stdout.strip().split("\n"):
                    print("  {}".format(line))
            return False
    except FileNotFoundError:
        print("FAILED: 'go' not found in PATH")
        return False
    except Exception as e:
        print("FAILED: {}".format(e))
        return False


def parse_args(argv=None):
    parser = argparse.ArgumentParser(
        description="rclone cross-compile packaging script",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Default targets
  python build-rclone.py

  # Custom targets
  python build-rclone.py --targets linux/amd64 windows/amd64

  # Custom output directory
  python build-rclone.py --out-dir dist

  # Keep debug symbols
  python build-rclone.py --no-strip

  # Verbose output
  python build-rclone.py --verbose
        """,
    )
    parser.add_argument(
        "--targets", "-t",
        nargs="+",
        default=["linux/amd64", "linux/arm64", "windows/386", "darwin/amd64"],
        help="Target platforms in os/arch format (default: linux/amd64 linux/arm64 windows/386 darwin/amd64)",
    )
    parser.add_argument(
        "--out-dir", "-o",
        default="build",
        help="Output directory for artifacts (default: build)",
    )
    parser.add_argument(
        "--version", "-v",
        default="",
        help="Version string; auto-read from VERSION file if empty",
    )
    parser.add_argument(
        "--tags",
        default="cmount",
        help="Go build tags (default: cmount, enables FUSE mount support on macOS/Windows)",
    )
    parser.add_argument(
        "--strip",
        action="store_true",
        default=True,
        dest="strip",
        help="Strip debug symbols (default: True)",
    )
    parser.add_argument(
        "--no-strip",
        action="store_false",
        dest="strip",
        help="Keep debug symbols in the binary",
    )
    parser.add_argument(
        "--verbose", "-V",
        action="store_true",
        default=False,
        help="Show detailed build commands and output",
    )
    parser.add_argument(
        "--list-targets",
        action="store_true",
        default=False,
        help="List all supported target platforms and exit",
    )
    return parser.parse_args(argv)


def list_targets():
    print("Supported target platforms (os/arch):")
    print()
    for t in ALL_TARGETS:
        goos, goarch = t.split("/", 1)
        arch_info = ""
        if goarch in ARCH_FLAGS or get_base_arch(goarch) in ARCH_FLAGS:
            flags = get_arch_flags(goarch)
            arch_info = "  [{}]".format(", ".join("{}={}".format(k, v) for k, v in flags.items()))
        print("  {:<25} {}".format(t, arch_info))
    print()

    # Group by OS
    by_os = {}
    for t in ALL_TARGETS:
        goos, goarch = t.split("/", 1)
        by_os.setdefault(goos, []).append(goarch)

    print("By OS:")
    for goos in sorted(by_os.keys()):
        archs = by_os[goos]
        print("  {:<12} {}".format(goos, " ".join(sorted(archs))))


if __name__ == "__main__":
    args = parse_args()

    if args.list_targets:
        list_targets()
        sys.exit(0)

    root_dir = os.path.dirname(os.path.abspath(__file__))
    version = args.version or get_version(root_dir)

    # Build ldflags
    #   --strip:    -s -w   (strip symbol table + DWARF, smallest binary)
    #   --no-strip: (none)  (keep all debug info, largest binary)
    ldflags = "-X github.com/rclone/rclone/fs.Version={}".format(version)
    if args.strip:
        ldflags = "-s -w {}".format(ldflags)

    # Create output directory
    out_dir = os.path.join(root_dir, args.out_dir) if not os.path.isabs(args.out_dir) else args.out_dir
    os.makedirs(out_dir, exist_ok=True)

    TARGETS = args.targets

    # Validate targets
    valid_targets = set(ALL_TARGETS)
    for t in TARGETS:
        if t not in valid_targets:
            print("Warning: '{}' is not in the known target list (use --list-targets to see all)".format(t))

    # Print banner
    print("=" * 50)
    print("  rclone Cross-Compile Script")
    print("  Version  : {}".format(version))
    print("  OutDir   : {}".format(out_dir))
    print("  Strip    : {}".format(args.strip))
    print("  Targets  : {}".format(", ".join(TARGETS)))
    print("=" * 50)
    print()

    success = 0
    failed = 0

    for i, target in enumerate(TARGETS, 1):
        parts = target.split("/")
        if len(parts) != 2:
            print("[{}/{}] Skipping invalid target '{}'".format(i, len(TARGETS), target))
            failed += 1
            continue
        goos, goarch = parts

        ok = build_target(goos, goarch, out_dir, ldflags, args.tags, args.verbose,
                              index=i, total=len(TARGETS))
        if ok:
            success += 1
        else:
            failed += 1

    # Summary
    print()
    print("=" * 50)
    print("  Build complete")
    print("  Success : {}".format(success))
    print("  Failed  : {}".format(failed))
    print("  OutDir  : {}".format(out_dir))
    print("=" * 50)

    if success > 0:
        print()
        print("Artifacts:")
        for f in sorted(os.listdir(out_dir)):
            fpath = os.path.join(out_dir, f)
            if os.path.isfile(fpath):
                size_mb = os.path.getsize(fpath) / (1024 * 1024)
                print("  {:<40} {:8.1f} MB".format(f, size_mb))

    sys.exit(1 if failed > 0 else 0)
