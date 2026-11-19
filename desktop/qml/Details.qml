import QtQuick
import QtQuick.Controls

Item {
    id: page

    property bool narrow: false

    signal goBack

    function contactStatus() {
        var all = (backend.state.contacts || []).concat(backend.state.requests || []);
        for (var i = 0; i < all.length; i++)
            if (all[i].pub === backend.detail)
                return all[i].status;
        return "";
    }

    Rectangle {
        id: header
        color: Theme.surface
        height: Theme.px(75)
        width: parent.width

        Rectangle {
            anchors.bottom: parent.bottom
            color: Theme.line
            height: 1
            width: parent.width
        }
        IconButton {
            id: back
            anchors.left: parent.left
            anchors.leftMargin: Theme.px(8)
            anchors.verticalCenter: parent.verticalCenter
            symbol: "back"
            tip: "Back to the list"
            visible: page.narrow

            onClicked: page.goBack()
        }
        Column {
            anchors.left: back.visible ? back.right : parent.left
            anchors.leftMargin: Theme.px(16)
            anchors.verticalCenter: parent.verticalCenter
            width: parent.width - Theme.px(40) - (back.visible ? back.width : 0)

            SLabel {
                elide: Text.ElideRight
                font.weight: Font.Bold
                objectName: "detailTitle"
                size: 18
                text: backend.detailTitle
                width: parent.width
            }
            SLabel {
                color: Theme.muted
                elide: Text.ElideRight
                size: 14
                text: backend.detailSubtitle
                width: parent.width
            }
        }
    }
    SScrollView {
        id: scroll
        anchors.bottom: parent.bottom
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: header.bottom
        clip: true
        contentWidth: availableWidth

        Column {
            id: column
            spacing: Theme.px(16)
            width: Math.min(Theme.px(800), scroll.availableWidth - Theme.px(48))
            x: (scroll.availableWidth - width) / 2
            y: Theme.px(24)

            Repeater {
                model: backend.cards

                delegate: InfoCard {
                    required property var modelData

                    item: modelData
                    width: column.width
                }
            }
            Rectangle {
                border.color: Theme.line
                color: Theme.surface
                height: editor.implicitHeight + Theme.px(44)
                radius: Theme.px(19)
                visible: backend.mode === "contacts" && backend.cards.length > 0
                width: parent.width

                SoftShadow {
                    anchors.fill: parent
                    rounding: parent.radius
                    z: -1
                }
                Column {
                    id: editor
                    spacing: Theme.px(10)
                    width: parent.width - Theme.px(44)
                    x: Theme.px(22)
                    y: Theme.px(20)

                    SLabel {
                        font.weight: Font.Bold
                        text: backend.detail === "self" ? "Edit your profile" : "Edit contact"
                    }
                    SLabel {
                        size: 14
                        text: backend.detail === "self" ? "Broadcast name" : "Your assigned name"
                    }
                    SInput {
                        objectName: "contactLabel"
                        text: backend.editLabel
                        width: parent.width

                        onTextEdited: backend.editLabel = text
                    }
                    SLabel {
                        size: 14
                        text: "Address"
                        visible: backend.detail !== "self"
                    }
                    SInput {
                        objectName: "contactAddress"
                        text: backend.editAddress
                        visible: backend.detail !== "self"
                        width: parent.width

                        onTextEdited: backend.editAddress = text
                    }
                    SLabel {
                        size: 14
                        text: "Notes"
                        visible: backend.detail !== "self"
                    }
                    SScrollView {
                        height: Theme.px(92)
                        visible: backend.detail !== "self"
                        width: parent.width

                        TextArea {
                            color: Theme.fg
                            font.family: Theme.family
                            font.pixelSize: Theme.px(16)
                            objectName: "contactNotes"
                            padding: Theme.px(12)
                            selectByMouse: true
                            text: backend.editNotes
                            wrapMode: TextEdit.Wrap

                            background: Rectangle {
                                border.color: parent.activeFocus ? Theme.primary : Theme.line
                                color: Theme.surface
                                radius: Theme.px(11)
                            }

                            onTextChanged: if (activeFocus)
                                backend.editNotes = text
                        }
                    }
                    Flow {
                        spacing: Theme.px(8)
                        width: parent.width

                        SButton {
                            enabled: !backend.saving
                            kind: "primary"
                            objectName: "contactSave"
                            text: backend.saving ? "Saving…" : "Save changes"

                            onClicked: backend.saveContact()
                        }
                        SButton {
                            text: "Open conversation"
                            visible: backend.detail !== "self" && page.contactStatus() === "accepted"

                            onClicked: backend.select(backend.detail)
                        }
                        SButton {
                            text: "Accept request"
                            visible: backend.detail !== "self" && page.contactStatus() === "pending"

                            onClicked: backend.contactAction(backend.detail, "accept")
                        }
                        SLabel {
                            color: Theme.muted
                            height: Theme.px(44)
                            size: 13
                            text: "Unsaved changes"
                            visible: backend.editing
                        }
                    }
                }
            }
            Item {
                height: Theme.px(24)
                width: 1
            }
        }
    }
}
