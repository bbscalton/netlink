# Netlink

Multi-line download accelerator for Windows by Neuereatec.

- Website: https://bbscalton.github.io/netlink/
- Windows installer (latest): https://github.com/bbscalton/netlink/releases/latest/download/Netlink-Setup-1.0.0.exe
- Releases: https://github.com/bbscalton/netlink/releases
- Diagram: https://bbscalton.github.io/neuereatec-services/netlink.html (if that page is not live yet, use the website above)

## 1. What Netlink is

Netlink is a rebranded build of **Plexo** by Anmol Kapil, released under the MIT License (https://github.com/anmolkapil/plexo). It is an Electron/TypeScript desktop app. The download engine is Plexo's, unchanged. The original LICENSE is kept in this repository.

## 2. Core claim

Netlink splits **one download** across **several network interfaces on the same PC**, for example Wi-Fi, Ethernet and USB phone tethering. Every part runs locally. There is no VPS, relay or proxy server.

**Each interface must be its own internet connection** (a different ISP, mobile data, and so on). Two adapters that reach the internet through the same router and uplink add no bandwidth.

## 3. How it works (from the source code)

- **Interface discovery:** `src/main/network/interfaces.ts` lists the OS adapters with Node's `os.networkInterfaces()` in `listActiveInterfaces()`. On Windows and macOS it also reads the adapters' friendly names.
- **Socket pinning:** `src/main/network/deviceBinding.ts` pins each TCP socket to one interface.
  - On Linux it uses `SO_BINDTODEVICE` through `setsockopt`.
  - Elsewhere, and as a fallback, it binds the socket's source IP with Node's `localAddress`.
- **Range requests:** files are fetched in pieces with HTTP Range requests, as defined in RFC 9110 §14 (https://www.rfc-editor.org/rfc/rfc9110#name-range-requests).
  - Blocks are 1 MiB to 8 MiB (`src/main/download/plan.ts`).
  - There are about 2 blocks for each possible stream, and they sit in a shared queue. Streams take a new block as soon as they finish one, so a faster line ends up fetching more blocks.
- **Adaptive streams:** each network starts with 8 streams. It can double up to 32 while every stream is receiving data (`plan.ts`, `concurrency.ts`).
- **Racing slow tail blocks:** near the end of a download, a slow block can be fetched a second time on another stream ("hedging", in `scheduler.ts`), and the first copy to finish wins.
- **Writing the file:** blocks are written at their own position in one staging (partial) file, which is renamed to the final file when the download is done. Resume state is saved, so a download can continue later.
- **Torrents:** each new peer connects through the network that has the fewest peers (`src/main/download/torrent/peers.ts`).

This is the same principle as multi-connection download managers such as IDM or XDM. The difference is that Netlink also spreads the connections across different interfaces.

## 4. What it does NOT do

- It does not bond or combine internet for a whole home or shop network, or for other devices.
- It does not speed up browsing, streaming, video calls, games or uploads. It only speeds up downloads that run inside the Netlink app.
- It does not help when the server does not support Range requests. Such a download runs as one stream on one line.
- It does not help when all the lines share one ISP uplink.
- Windows may keep sending traffic over its preferred route or leave a line idle. The results depend on how Windows routes traffic.

## 5. OpenWrt / router port

This repository does not include a router version. A port looks possible: the engine is Node/TypeScript, and on Linux it uses `SO_BINDTODEVICE`. It would need to be separated from Electron and run as a Linux service on the router. Even then, it would only speed up downloads that the service itself performs.

## Install

The installer is not code-signed. If Windows SmartScreen shows "Windows protected your PC", click **More info → Run anyway**.

## Build

```
npm ci
npm run build:win
```

## Credits

Based on Plexo by Anmol Kapil (MIT): https://github.com/anmolkapil/plexo
