# planeterm

A terminal-mode [PPI scope](https://en.wikipedia.org/wiki/Plan_position_indicator) that plots live aircraft from a `dump1090` ADS-B
feed. Green CRT aesthetic, range rings, every known contact shown at once
with callsign, altitude and distance.

![planeterm](ppi.gif)

## Real ADS-B (macOS & Linux)

You need an RTL-SDR USB dongle (the cheap `RTL2832U` ones work). Everything
else is in the Makefile:

```
make deps           # librtlsdr + pkg-config (Homebrew or your distro's package manager)
make dump1090       # clone & build FlightAware's dump1090 into external/
make run-dump1090   # leave this running in one terminal
make run            # in another terminal — the radar
```

`make run` and `make run-dump1090` honor `RADAR_LAT`, `RADAR_LON`,
`RADAR_RANGE`, and `SBS_PORT` so you can drop them in front:

```
RADAR_LAT=51.47 RADAR_LON=-0.4543 make run
```

Once the dongle is plugged in, `rtl_test -t` should report something like
`Found 1 device(s): 0: Realtek, RTL2838UHIDIR`. If it can't claim the
device, unplug-replug usually fixes it on macOS.

**Linux gotcha:** the kernel's DVB-T driver auto-claims the RTL2832U on
boot, which makes librtlsdr report "no supported devices found." Blacklist
the kernel module once and reboot (or `rmmod`):

```
sudo tee /etc/modprobe.d/no-rtl-dvb.conf >/dev/null <<'EOF'
blacklist dvb_usb_rtl28xxu
blacklist rtl2832
blacklist rtl2830
EOF
sudo rmmod dvb_usb_rtl28xxu 2>/dev/null || true
```

Also add yourself to a group that can access the USB device (e.g. `plugdev`
on Debian/Ubuntu) or install the udev rules that ship with `rtl-sdr`.

### What the targets do

| Target            | What it does                                                  |
| ----------------- | ------------------------------------------------------------- |
| `make` / `build`    | Compile `./planeterm`.                                                                  |
| `make deps`         | Install `librtlsdr` + `pkg-config` (Homebrew on macOS; apt / dnf / pacman / zypper / apk on Linux). |
| `make dump1090`     | Clone FlightAware's fork into `external/dump1090` and `make` it.                        |
| `make run-dump1090` | Run the locally-built dump1090 with `--net --net-sbs-port 30003`.                       |
| `make run`          | Run planeterm against `localhost:30003`.                                                |
| `make doctor`       | Diagnose why the SDR isn't detected (USB tree, drivers, kernel modules, udev, groups).  |
| `make clean`        | Remove the planeterm binary.                                                            |
| `make clean-all`    | Also wipe `external/`.                                                                  |

On macOS the Makefile auto-detects Homebrew's prefix (`/opt/homebrew` on
Apple Silicon, `/usr/local` on Intel) and threads it through
`PKG_CONFIG_PATH` so the dump1090 build finds `librtlsdr` without any
manual env work. On Linux, `pkg-config` already knows where the distro put
the library.

`make doctor` is cross-platform: it picks the right diagnostics based on
`uname -s`. On Linux the most common gotcha it surfaces is the
`dvb_usb_rtl28xxu` kernel module having claimed the dongle — see the
blacklist snippet above.

## Flags

| Flag              | Default              | Description                                      |
| ----------------- | -------------------- | ------------------------------------------------ |
| `-host`           | `localhost`          | dump1090 SBS host                                |
| `-port`           | `30003`              | dump1090 SBS port                                |
| `-lat`            | `49.699070`          | radar center latitude (degrees)                  |
| `-lon`            | `11.953948`          | radar center longitude (degrees)                 |
| `-range`          | `100`                | radar range in nautical miles                    |
| `-ttl`            | `60s`                | drop aircraft after this long with no update     |
| `-highlight`      | `highlight.yaml`     | callsign-based highlight rules (missing OK)      |
| `-mil-file`       | `mil.yaml`           | military ICAO hex ranges (missing OK)            |

Env-var equivalents: `DUMP1090_HOST`, `DUMP1090_PORT`, `RADAR_LAT`,
`RADAR_LON`, `RADAR_RANGE_NM`, `HIGHLIGHT_FILE`, `MIL_FILE`.

## Keys

- `q` / `Esc` / `Ctrl-C` — quit
- `+` / `=` — zoom in (smaller range)
- `-` / `_` — zoom out (larger range)

## Features

- Highlight aircraft by callsign prefix (see `highlight.yaml`).
- Highlight aircraft by ICAO 24-bit address against well-known military
  hex ranges (see `mil.yaml`). The shipped file covers the commonly cited
  US + NATO allocations and is loaded by default; any highlight rule with
  `mil: true` will fire for matching aircraft. Edit or replace it via
  `-mil-file` to taste.

## How it works

- `internal/sbs` opens a TCP connection to dump1090's SBS-1 text feed
  (port 30003) and parses each `MSG,…` line, merging callsign / altitude /
  position / track / speed into the per-ICAO record.
- `internal/aircraft` is a thread-safe store with a TTL so stale tracks
  fall off the scope automatically.
- `internal/radar` draws the scope with [tcell](https://github.com/gdamore/tcell).
  For every aircraft it computes great-circle distance and bearing to the
  radar center and projects the position onto the scope.

## License

MIT