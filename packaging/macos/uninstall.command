#!/bin/sh
set -eu

PRODUCT_NAME="QianuHub闲鱼助手"
EXPECTED_BUNDLE_ID="com.qianuhub.xianyu-helper"
CONSOLE_USER="$(/usr/bin/stat -f '%Su' /dev/console)"

if [ "${1:-}" != "--confirmed" ]; then
  if ! /usr/bin/osascript <<'APPLESCRIPT'
tell application "System Events"
    display dialog "卸载后将删除 QianuHub闲鱼助手、后台服务、配置、数据库、Chromium 和日志。此操作不可恢复。" buttons {"取消", "卸载"} default button "取消" cancel button "取消" with title "卸载 QianuHub闲鱼助手"
end tell
APPLESCRIPT
  then
    exit 0
  fi
fi

if [ "$(id -u)" -ne 0 ]; then
  exec /usr/bin/sudo "$0" --confirmed
fi

if [ "$CONSOLE_USER" != "root" ] && [ -n "$CONSOLE_USER" ]; then
  UID_VALUE="$(/usr/bin/id -u "$CONSOLE_USER")"
  HOME_DIR="$(/usr/bin/dscl . -read "/Users/$CONSOLE_USER" NFSHomeDirectory | /usr/bin/awk '{print $2}')"

  for label in \
    "com.qianuhub.xianyu-helper.server" \
    "com.qianuhub.xianyu-helper.tray" \
    "com.christ.qianuhub-xianyu-helper.server" \
    "com.christ.qianuhub-xianyu-helper.tray"
  do
    /bin/launchctl bootout "gui/$UID_VALUE/$label" >/dev/null 2>&1 || true
  done

  # 处理用户手动启动、但没有被 LaunchAgent 管理的旧进程。
  for process_path in \
    "/Applications/QianuHub闲鱼助手/QianuHub闲鱼助手.app/Contents/Helpers/xianyu-server" \
    "/Applications/QianuHub闲鱼助手/QianuHub闲鱼助手.app/Contents/MacOS/QianuHub闲鱼助手" \
    "/Applications/QianuHub Xianyu Helper.app/Contents/Helpers/xianyu-server" \
    "/Applications/QianuHub Xianyu Helper.app/Contents/MacOS/xianyu-tray" \
    "/Applications/QianuHub Xianyu Helper.localized/QianuHub Xianyu Helper.app/Contents/Helpers/xianyu-server" \
    "/Applications/QianuHub Xianyu Helper.localized/QianuHub Xianyu Helper.app/Contents/MacOS/xianyu-tray"
  do
    /usr/bin/pkill -TERM -f "$process_path" >/dev/null 2>&1 || true
  done

  /bin/rm -f \
    "$HOME_DIR/Library/LaunchAgents/com.qianuhub.xianyu-helper.server.plist" \
    "$HOME_DIR/Library/LaunchAgents/com.qianuhub.xianyu-helper.tray.plist" \
    "$HOME_DIR/Library/LaunchAgents/com.christ.qianuhub-xianyu-helper.server.plist" \
    "$HOME_DIR/Library/LaunchAgents/com.christ.qianuhub-xianyu-helper.tray.plist"

  /bin/rm -rf \
    "$HOME_DIR/Library/Application Support/QianuHubXianyuHelper" \
    "$HOME_DIR/Library/Logs/QianuHubXianyuHelper"
fi

/bin/rm -rf \
  "/Applications/QianuHub闲鱼助手" \
  "/Applications/QianuHub Xianyu Helper.app" \
  "/Applications/QianuHub Xianyu Helper.localized"
/usr/bin/pkgutil --forget "$EXPECTED_BUNDLE_ID" >/dev/null 2>&1 || true
/bin/rm -f /var/log/qianuhub-xianyu-helper-install.log

echo "$PRODUCT_NAME 已卸载完成。"
