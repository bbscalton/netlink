# OpenWrt SDK package (builds with the SDK's Go toolchain: needs feeds/packages lang/golang)
include $(TOPDIR)/rules.mk

PKG_NAME:=netlink
PKG_VERSION:=0.1.0
PKG_RELEASE:=1
PKG_LICENSE:=MIT
PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_BUILD_FLAGS:=no-mips16

GO_PKG:=netlink-openwrt
GO_PKG_BUILD_PKG:=netlink-openwrt/cmd/netlink
GO_PKG_LDFLAGS_X:=main.version=$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

define Package/netlink
  SECTION:=net
  CATEGORY:=Network
  TITLE:=Multi-WAN HTTP range downloader (Netlink)
  DEPENDS:=$(GO_ARCH_DEPENDS)
endef

define Package/netlink/description
  Splits HTTP downloads into byte ranges fetched over several WAN interfaces at once
  (SO_BINDTODEVICE per line) and reassembles them on disk. Web UI on port 8099.
endef

define Package/netlink/conffiles
/etc/config/netlink
endef

define Build/Prepare
	mkdir -p $(PKG_BUILD_DIR)
	$(CP) ./src/* $(PKG_BUILD_DIR)/
endef

define Package/netlink/install
	$(call GoPackage/Package/Install/Bin,$(1))
	$(INSTALL_DIR) $(1)/etc/init.d $(1)/etc/config
	$(INSTALL_BIN) ./files/netlink.init $(1)/etc/init.d/netlink
	$(INSTALL_CONF) ./files/netlink.config $(1)/etc/config/netlink
endef

$(eval $(call GoBinPackage,netlink))
$(eval $(call BuildPackage,netlink))
