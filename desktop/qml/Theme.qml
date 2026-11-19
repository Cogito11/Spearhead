pragma Singleton
import QtQuick

QtObject {
    readonly property real scale: backend.fontSize / 100
    readonly property bool dark: backend.theme === "dark" || (backend.theme === "system" && backend.systemDark)
    readonly property string family: "Noto Sans"
    readonly property color bg: dark ? "#0c1423" : "#eef2f8"
    readonly property color surface: dark ? "#16243a" : "#ffffff"
    readonly property color sidebar: dark ? "#111d30" : "#f7f9fd"
    readonly property color fg: dark ? "#edf3fc" : "#17213a"
    readonly property color muted: dark ? "#abbcd2" : "#576980"
    readonly property color line: dark ? "#2d405a" : "#d2dce9"
    readonly property color primary: dark ? "#94c7f5" : "#0b5cad"
    readonly property color primaryText: dark ? "#0d1b2f" : "#ffffff"
    readonly property color soft: dark ? "#1b3656" : "#e5effb"
    readonly property color danger: dark ? "#ff9699" : "#a4161a"
    readonly property color ok: dark ? "#7bddaa" : "#0f6b2f"
    readonly property color warn: dark ? "#ffcc85" : "#8a4b00"
    readonly property color bubble: dark ? "#1a2a42" : "#ffffff"
    function px(value) {
        return value * scale;
    }
}
