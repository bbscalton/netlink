#!/bin/sh
# Cross-compiles static binaries and wraps them in .ipk files (opkg, OpenWrt <= 24.10).
set -e
cd "$(dirname "$0")"
VER=0.1.0
rm -rf dist && mkdir -p dist
# openwrt-arch  GOARCH  extra-env
TARGETS="mipsel_24kc:mipsle:GOMIPS=softfloat
mips_24kc:mips:GOMIPS=softfloat
arm_cortex-a7_neon-vfpv4:arm:GOARM=7
arm_cortex-a9:arm:GOARM=7
aarch64_cortex-a53:arm64:
aarch64_generic:arm64:
x86_64:amd64:"
for t in $TARGETS; do
  A=${t%%:*}; rest=${t#*:}; GA=${rest%%:*}; EX=${rest#*:}
  B=dist/netlink-$A
  env CGO_ENABLED=0 GOOS=linux GOARCH=$GA $EX go build -trimpath -ldflags "-s -w -X main.version=$VER" -o $B ./cmd/netlink
  W=$(mktemp -d); mkdir -p $W/data/usr/bin $W/data/etc/init.d $W/data/etc/config $W/control
  cp $B $W/data/usr/bin/netlink
  cp files/netlink.init $W/data/etc/init.d/netlink; chmod 755 $W/data/etc/init.d/netlink $W/data/usr/bin/netlink
  cp files/netlink.config $W/data/etc/config/netlink
  SZ=$(du -sk $W/data | cut -f1)
  cat > $W/control/control <<C
Package: netlink
Version: $VER-1
Depends: libc
Architecture: $A
Installed-Size: $((SZ*1024))
License: MIT
Section: net
Maintainer: Calton Andrew
Description: Netlink multi-WAN HTTP range downloader (web UI on :8099). Not yet tested on real hardware.
C
  echo /etc/config/netlink > $W/control/conffiles
  printf '#!/bin/sh\n[ -n "$IPKG_INSTROOT" ] || /etc/init.d/netlink enable\nexit 0\n' > $W/control/postinst
  printf '#!/bin/sh\n[ -n "$IPKG_INSTROOT" ] || { /etc/init.d/netlink stop; /etc/init.d/netlink disable; }\nexit 0\n' > $W/control/prerm
  chmod 755 $W/control/postinst $W/control/prerm
  echo 2.0 > $W/debian-binary
  (cd $W/control && tar --owner=0 --group=0 -czf ../control.tar.gz ./)
  (cd $W/data && tar --owner=0 --group=0 -czf ../data.tar.gz ./)
  (cd $W && tar --owner=0 --group=0 -czf pkg.ipk ./debian-binary ./control.tar.gz ./data.tar.gz)
  (cd $W/data && tar --owner=0 --group=0 -czf ../tgz ./)
  mv $W/tgz dist/netlink-openwrt_${VER}_$A.tar.gz
  mv $W/pkg.ipk dist/netlink_${VER}-1_$A.ipk; rm -rf $W
done
(cd dist && sha256sum * > SHA256SUMS)
ls -l dist
