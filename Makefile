GO ?= go

EXTERNAL_DIR  := external
DUMP1090_DIR  := $(EXTERNAL_DIR)/dump1090
DUMP1090_REPO ?= https://github.com/flightaware/dump1090.git
DUMP1090_BIN  := $(DUMP1090_DIR)/dump1090

PLANETERM_BIN := planeterm

# Overridable runtime defaults
RADAR_LAT   ?= 49.699070
RADAR_LON   ?= 11.953948
RADAR_RANGE ?= 100
SBS_PORT    ?= 30003

# Help pkg-config find Homebrew on both Apple Silicon (/opt/homebrew) and
# Intel (/usr/local) when building dump1090..
BREW_PREFIX := $(shell command -v brew >/dev/null 2>&1 && brew --prefix 2>/dev/null)
ifneq ($(BREW_PREFIX),)
export PKG_CONFIG_PATH := $(BREW_PREFIX)/lib/pkgconfig:$(PKG_CONFIG_PATH)
endif

GO_SOURCES := $(shell find . -name '*.go' -not -path './$(EXTERNAL_DIR)/*' 2>/dev/null)

.PHONY: build dump1090 deps run run-dump1090 clean clean-all help doctor doctor-darwin doctor-linux
.DEFAULT_GOAL := build

## build         Build the planeterm binary (default).
build: $(PLANETERM_BIN)

$(PLANETERM_BIN): go.mod $(GO_SOURCES)
	$(GO) build -o $@ .

## deps          Install librtlsdr + pkg-config (macOS via Homebrew, Linux via apt/dnf/pacman/zypper).
deps:
	@case "$$(uname -s)" in \
		Darwin) \
			command -v brew >/dev/null || { echo "Homebrew required: https://brew.sh"; exit 1; }; \
			brew install librtlsdr pkg-config ;; \
		Linux) \
			if command -v apt-get >/dev/null; then \
				sudo apt-get update && sudo apt-get install -y librtlsdr-dev rtl-sdr pkg-config build-essential git; \
			elif command -v dnf >/dev/null; then \
				sudo dnf install -y rtl-sdr-devel rtl-sdr pkgconf-pkg-config gcc make git; \
			elif command -v pacman >/dev/null; then \
				sudo pacman -S --needed --noconfirm rtl-sdr pkgconf base-devel git; \
			elif command -v zypper >/dev/null; then \
				sudo zypper install -y rtl-sdr-devel rtl-sdr pkg-config gcc make git; \
			elif command -v apk >/dev/null; then \
				sudo apk add --no-cache rtl-sdr rtl-sdr-dev pkgconf build-base git; \
			else \
				echo "Unsupported Linux distro. Install librtlsdr-dev, rtl-sdr, pkg-config, and a C toolchain manually."; exit 1; \
			fi ;; \
		*) echo "Unsupported OS: $$(uname -s)"; exit 1 ;; \
	esac

## dump1090      Clone and build FlightAware's dump1090 into external/dump1090.
dump1090: $(DUMP1090_BIN)

$(DUMP1090_DIR)/.git:
	@mkdir -p $(EXTERNAL_DIR)
	git clone --depth=1 $(DUMP1090_REPO) $(DUMP1090_DIR)

$(DUMP1090_BIN): $(DUMP1090_DIR)/.git
	@command -v pkg-config >/dev/null || { echo "pkg-config missing — run 'make deps' first"; exit 1; }
	@pkg-config --exists librtlsdr || { echo "librtlsdr missing — run 'make deps' first"; exit 1; }
	$(MAKE) -C $(DUMP1090_DIR)

## run-dump1090  Run the locally-built dump1090 with the SBS feed on tcp/30003.
run-dump1090: dump1090
	$(DUMP1090_BIN) --net --net-sbs-port $(SBS_PORT) --lat $(RADAR_LAT) --lon $(RADAR_LON) --quiet

## run           Run planeterm against localhost:30003.
run: build
	./$(PLANETERM_BIN) -lat $(RADAR_LAT) -lon $(RADAR_LON) -range $(RADAR_RANGE) -port $(SBS_PORT)

## clean         Remove the planeterm binary.
clean:
	rm -f $(PLANETERM_BIN)

## clean-all     Also remove external/ (the dump1090 source/build tree).
clean-all: clean
	rm -rf $(EXTERNAL_DIR)

## doctor        Diagnose RTL-SDR detection (macOS or Linux)
doctor:
	@case "$$(uname -s)" in \
		Darwin) $(MAKE) --no-print-directory doctor-darwin ;; \
		Linux)  $(MAKE) --no-print-directory doctor-linux ;; \
		*) echo "Unsupported OS: $$(uname -s)"; exit 1 ;; \
	esac

doctor-darwin:
	@echo "== shell architecture =="
	@printf "  arch:  "; arch
	@printf "  uname: "; uname -m
	@echo
	@echo "== Homebrew =="
	@if command -v brew >/dev/null; then \
		echo "  brew prefix: $$(brew --prefix)"; \
	else echo "  brew NOT INSTALLED — https://brew.sh"; fi
	@echo
	@echo "== librtlsdr install =="
	@if brew --prefix librtlsdr >/dev/null 2>&1; then \
		p=$$(brew --prefix librtlsdr); echo "  installed: $$p"; \
		ls -1 $$p/lib/librtlsdr*.dylib 2>/dev/null | sed 's/^/    /'; \
	else echo "  NOT INSTALLED — run 'make deps'"; fi
	@echo
	@echo "== USB enumeration =="
	@if system_profiler SPUSBDataType 2>/dev/null | grep -iqE "realtek|rtl28[0-9]+"; then \
		system_profiler SPUSBDataType 2>/dev/null | awk '/Realtek|RTL28/{flag=8} flag{print "  " $$0; flag--}'; \
	else \
		echo "  no Realtek RTL28xx device found in USB tree"; \
		echo "  → unplug & replug, try another port, avoid hubs/adapters"; \
		echo "  → some clones use a non-standard VID/PID and won't be recognized"; \
	fi
	@echo
	@echo "== rtl_test (5 sec) =="
	@if command -v rtl_test >/dev/null; then \
		rtl_test -t 2>&1 | sed 's/^/  /' | head -12 || true; \
	else \
		alt=$$(brew --prefix librtlsdr 2>/dev/null)/bin/rtl_test; \
		if [ -x "$$alt" ]; then \
			$$alt -t 2>&1 | sed 's/^/  /' | head -12 || true; \
		else echo "  rtl_test missing — run 'make deps'"; fi; \
	fi
	@echo
	@echo "== conflicting kexts (should be empty) =="
	@out=$$(kextstat 2>/dev/null | grep -iE "rtl28|rtlsdr|dvbt|driver\.usb\.dvb" || true); \
		if [ -n "$$out" ]; then echo "$$out" | sed 's/^/  /'; else echo "  none"; fi

doctor-linux:
	@echo "== kernel & arch =="
	@printf "  uname: "; uname -srm
	@if [ -r /etc/os-release ]; then printf "  distro: "; . /etc/os-release && echo "$$PRETTY_NAME"; fi
	@echo
	@echo "== librtlsdr =="
	@if command -v rtl_test >/dev/null; then echo "  rtl_test: $$(command -v rtl_test)"; \
		else echo "  rtl_test missing — run 'make deps'"; fi
	@if command -v pkg-config >/dev/null && pkg-config --exists librtlsdr 2>/dev/null; then \
		echo "  librtlsdr: $$(pkg-config --modversion librtlsdr)"; \
	else echo "  librtlsdr-dev missing — run 'make deps'"; fi
	@echo
	@echo "== USB enumeration =="
	@if command -v lsusb >/dev/null; then \
		out=$$(lsusb | grep -iE "realtek|rtl28|0bda:" || true); \
		if [ -n "$$out" ]; then echo "$$out" | sed 's/^/  /'; \
		else \
			echo "  no Realtek RTL28xx device found in lsusb"; \
			echo "  → unplug & replug, try another port, avoid hubs/adapters"; \
			echo "  → check 'dmesg | tail -30' for USB attach events"; \
		fi; \
	else echo "  lsusb missing — install the 'usbutils' package"; fi
	@echo
	@echo "== rtl_test (5 sec) =="
	@if command -v rtl_test >/dev/null; then \
		rtl_test -t 2>&1 | sed 's/^/  /' | head -12 || true; \
	else echo "  rtl_test missing — run 'make deps'"; fi
	@echo
	@echo "== conflicting kernel modules =="
	@out=$$(lsmod 2>/dev/null | awk '$$1 ~ /^(dvb_usb_rtl28xxu|rtl2832|rtl2830|dvb_core)$$/'); \
		if [ -n "$$out" ]; then \
			echo "$$out" | sed 's/^/  /'; \
			echo "  → these hijack the dongle; see README for the blacklist file"; \
		else echo "  none loaded — good"; fi
	@echo
	@echo "== blacklist files (looking for rtl28xxu / dvb_usb entries) =="
	@hits=$$(grep -lE "rtl28|dvb_usb" /etc/modprobe.d/*.conf /etc/modprobe.conf 2>/dev/null || true); \
		if [ -n "$$hits" ]; then echo "$$hits" | sed 's/^/  /'; else echo "  none — see README to add one"; fi
	@echo
	@echo "== udev rules for rtl-sdr =="
	@hits=$$(ls /lib/udev/rules.d/ /usr/lib/udev/rules.d/ /etc/udev/rules.d/ 2>/dev/null | grep -i rtl | sort -u); \
		if [ -n "$$hits" ]; then echo "$$hits" | sed 's/^/  /'; else echo "  no rtl-sdr udev rules found — non-root users may not be able to claim the device"; fi
	@echo
	@echo "== current user groups =="
	@printf "  "; id -nG 2>/dev/null

## help          Show this help.
help:
	@awk '/^## / { sub(/^## /, "  "); print }' $(MAKEFILE_LIST)
