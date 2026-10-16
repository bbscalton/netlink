# Netlink for OpenWrt (v0.1.0, not yet tested on real hardware)

Netlink downloads different parts of the same file through each internet connection on the router, then assembles them on disk. It is a port of the Netlink/Plexo HTTP engine to a single static Go binary for stock OpenWrt: no firmware rebuild, no VPS, no tunnel, no MPTCP, no bonded IP. LAN devices paste a link into the router's web page (port 8099) and save the finished file from there.

**Status:** built and tested on a Linux PC (checksums match). Not yet tested on a real router; manual router testing is the next step.

## How it works (same as Netlink)
1. Probe: `GET` with `Range: bytes=0-0`. A `206` with `Content-Range: bytes 0-0/N` means the file can be split; anything else is one stream.
2. Plan: the file is cut into blocks of 1–8 MiB (about 2 blocks per stream, as in Netlink's `plan.ts`).
3. Bind: each WAN line runs N streams (default 4). Every TCP socket is pinned to that line's device with `SO_BINDTODEVICE` (and optionally its source IP), so it leaves through that line with its own public IP.
4. Fetch: streams pull blocks from a shared queue, so a faster line takes more blocks. Each `206` reply's `Content-Range` is checked. A failed block goes back to the queue, and a line that keeps failing drops out. Near the end, an idle stream may race the slowest block once (a "hedge", as in Netlink's scheduler).
5. Assemble: bytes are written at their offsets (`pwrite`) into one pre-sized `file.part`, which is fsynced and renamed when every block is complete.

## Requirements
- OpenWrt with **each line on its own WAN interface** (e.g. `wan` on `eth1`, `wan2` on `usb0`/`eth2`), each with its own default gateway/route. Not pre-bonded or load-balanced into one interface. mwan3 is optional; Netlink binds per device itself. Each WAN needs a route in the main table (or the table mwan3 leaves) for that device. Give each WAN a different `metric` so both stay up.
- Kernel ≥ 5.7 for unprivileged `SO_BINDTODEVICE` (the service runs as root anyway).
- Around 15 MB free RAM while running. Flash: about 2.5 MB for the package (the binary is 6–7 MB unpacked; see the sizes below). If flash is tight, run the binary from USB.
- Storage: `/tmp/netlink` (RAM) by default. **For anything bigger than a few hundred MB, point `dir` at a USB drive** (`/mnt/sda1/netlink`).

## Pick your package
Find your arch with `cat /etc/openwrt_release | grep ARCH` (or `opkg print-architecture` / `apk --print-arch`).

| OpenWrt arch | binary | .ipk |
|---|---|---|
| mipsel_24kc (MT7621, MT76x8) | 7.2 MB | 2.5 MB |
| mips_24kc (ath79) | 7.2 MB | 2.6 MB |
| arm_cortex-a7_neon-vfpv4 (ipq40xx) | 6.4 MB | 2.6 MB |
| arm_cortex-a9 (mvebu) | 6.4 MB | 2.6 MB |
| aarch64_cortex-a53 / aarch64_generic (filogic, RPi, rockchip) | 6.2 MB | 2.5 MB |
| x86_64 | 6.5 MB | 2.7 MB |

## Install
**OpenWrt ≤ 24.10 (opkg):**
```sh
scp netlink_0.1.0-1_<arch>.ipk root@router:/tmp/
opkg install /tmp/netlink_0.1.0-1_<arch>.ipk
```
**OpenWrt 25.12+ (apk), or any release:** use the plain tarball (binary + init script + config):
```sh
scp netlink-openwrt_0.1.0_<arch>.tar.gz root@router:/tmp/
tar -xzf /tmp/netlink-openwrt_0.1.0_<arch>.tar.gz -C /
/etc/init.d/netlink enable
```
(A native `.apk` needs the OpenWrt SDK: `package/netlink/` is an SDK package that builds both formats.)

## Configure wan1..wanN
Find each line's device: `ifstatus wan | grep l3_device`, `ifstatus wan2 | grep l3_device`.
```sh
uci set netlink.wan1.device='eth1'
uci set netlink.wan2.device='usb0'
uci add netlink wan; uci rename netlink.@wan[-1]=wan3; uci set netlink.wan3.device='wwan0'   # more lines
uci set netlink.main.dir='/mnt/sda1/netlink'
uci commit netlink && /etc/init.d/netlink restart
```
Options per `wan` section: `device` (required), `source` (local IP, optional), `streams` (default 4), `enabled`.
Then open `http://<router-lan-ip>:8099`, paste a URL, tick the lines, and click **Save to this device** when it's done.

Shortcut from any LAN device: `http://<router>:8099/get?url=<file-url>` downloads over all lines, then serves the file. Check with: `logread -e netlink`.

## CLI
```sh
netlink -i wan1=eth1,wan2=usb0:6 -o /mnt/sda1/file.iso https://example.com/file.iso
netlink -listen 0.0.0.0:8099 -dir /mnt/sda1/netlink -i wan1=eth1,wan2=usb0
```
Interface syntax: `name=device[@sourceIP][:streams]`.

## Limitations
- It only speeds up downloads made **through Netlink** (web UI, `/get`, CLI). Normal browsing, streaming and speed tests still use one line.
- The server must support HTTP Range requests (most CDNs and mirrors do). Without them it falls back to one stream on one line.
- Links that need browser cookies/login don't work unless the URL itself carries the token.
- No resume after a router reboot yet. The UI has no password: keep port 8099 on the LAN only (the default firewall already blocks it from WAN).
- A file is first fully stored on the router, then copied to the LAN device.

## Build
`./build.sh` (Go ≥ 1.22) → `dist/` with static binaries, `.ipk` and tarballs for each arch, plus `SHA256SUMS`.
SDK: copy `package/netlink` into the SDK's `package/`, `./scripts/feeds install golang`, `make package/netlink/compile`.

## Tests done (on a Linux PC, not a router)
- Alpine 3.20.3 ISO (219 MB) over two bound streams on the same NIC: sha256 matches the published one, 3 runs.
- One dead interface (down `docker0`) beside a working one: its blocks failed over to the working line; checksum OK.
- Server without Range support (python http.server): single-stream fallback; checksum OK.
- Web UI, job API and `/get`: checksum OK.

License: MIT (Netlink/Plexo by Anmol Kapil).
