import QtQuick

Text {
    property real size: 16
    font.family: Theme.family
    font.pixelSize: Theme.px(size)
    color: Theme.fg
    textFormat: Text.PlainText
    renderType: Text.NativeRendering
    lineHeightMode: Text.FixedHeight
    lineHeight: Theme.px(size * 1.5)
    verticalAlignment: Text.AlignVCenter
}
