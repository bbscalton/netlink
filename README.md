# Netlink

**Netlink** by Neuereatec is a multi-line download accelerator for Windows. It splits each download into pieces and fetches them in parallel over every internet connection on your PC (e.g. Wi-Fi + Ethernet + USB tethering), combining their speed.

- Each line must be its own internet connection.
- Purely local: no server or VPS needed.
- Only speeds up downloads done inside the app.

Download: [latest Windows installer](https://github.com/bbscalton/netlink/releases/latest) · Website: https://bbscalton.github.io/netlink/

The installer is unsigned: Windows SmartScreen may warn — click **More info → Run anyway**.

## Build
```
npm ci
npm run build:win
```

## Credits
Based on Plexo by Anmol Kapil (MIT) — https://github.com/anmolkapil/plexo. See [LICENSE](LICENSE).
