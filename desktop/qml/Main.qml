import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import QtQuick.Dialogs as SystemDialogs

ApplicationWindow {
    id: root

    property var contextMessage: ({})
    property string contextPub: ""
    property string contextSelection: ""
    property string contextType: ""
    property bool detailShown: backend.current.length > 0
    property string downloadId: ""
    property bool narrow: width < 760
    property bool newerMessages: false
    property bool showSidebar: !narrow || !detailShown
    property bool toastError: false
    property string toastText: ""
    property bool wasAtBottom: true

    function choose(key) {
        if (backend.mode === "conversations")
            backend.select(key);
        else {
            backend.showDetail(key);
            detailShown = true;
        }
    }
    function contactMenu(pub, px, py) {
        contextType = backend.mode === "network" ? "device" : "contact";
        contextPub = pub;
        contextMenu.x = Math.max(8, Math.min(px, width - contextMenu.width - 8));
        contextMenu.y = Math.max(8, Math.min(py, height - contextMenu.implicitHeight - 8));
        contextMenu.open();
    }
    function filteredRequests() {
        var query = backend.sidebarSearch.trim().toLowerCase();
        return (backend.state.requests || []).filter(function (c) {
                return !query || [c.label, c.their_name, c.device, c.fingerprint].join(" ").toLowerCase().indexOf(query) >= 0;
            });
    }
    function messageMenu(message, px, py, selection) {
        contextType = "message";
        contextMessage = message;
        contextSelection = selection;
        contextPub = backend.current;
        contextMenu.x = Math.max(8, Math.min(px, width - contextMenu.width - 8));
        contextMenu.y = Math.max(8, Math.min(py, height - contextMenu.implicitHeight - 8));
        contextMenu.open();
    }
    function noSearchResults() {
        return backend.sidebarSearch.trim().length > 0 && (backend.mode === "conversations" ? people.count === 0 && filteredRequests().length === 0 : people.count <= 1);
    }
    function openViewMenu(selector) {
        viewMenu.parent = selector;
        viewMenu.x = 0;
        viewMenu.y = selector.height + Theme.px(6);
        viewMenu.width = selector.width;
        viewMenu.open();
    }
    function saveAttachment(id, name) {
        downloadId = id;
        downloadDialog.currentFile = backend.suggestedFile(name);
        downloadDialog.open();
    }

    color: Theme.bg
    font.family: Theme.family
    font.pixelSize: Theme.px(16)
    height: initialHeight
    minimumHeight: 400
    minimumWidth: 400
    objectName: "spearheadWindow"
    title: "SpearHead"
    visible: !runtimeCheck
    width: initialWidth

    onActiveChanged: backend.setWindowActive(active)
    onClosing: function (close) {
        close.accepted = false;
        backend.closeWindow(width, height);
    }

    Shortcut {
        enabled: backend.current.length > 0 && !dialogs.anyOpen
        sequence: StandardKey.Find

        onActivated: {
            backend.searchOpen = true;
            searchInput.forceActiveFocus();
        }
    }
    Shortcut {
        enabled: backend.searchOpen && !dialogs.anyOpen
        sequence: "Escape"

        onActivated: backend.searchOpen = false
    }
    Connections {
        function onDetailSelected() {
            root.detailShown = true;
        }
        function onMessagesChanged(reset) {
            Qt.callLater(function () {
                    if (reset || root.wasAtBottom) {
                        if (backend.searchOpen && backend.search.trim().length > 0)
                            chatLog.positionViewAtBeginning();
                        else
                            chatLog.positionViewAtEnd();
                        root.newerMessages = false;
                    } else
                        root.newerMessages = true;
                });
        }
        function onMessagesChanging(reset) {
            root.wasAtBottom = chatLog.atYEnd;
        }
        function onNavigationChanged() {
            if (backend.mode === "conversations")
                root.detailShown = backend.current.length > 0;
            else
                root.detailShown = false;
        }
        function onToast(text, error) {
            root.toastText = text;
            root.toastError = error;
            toastTimer.restart();
        }

        target: backend
    }
    ColumnLayout {
        anchors.fill: parent
        spacing: 0

        Rectangle {
            Layout.fillWidth: true
            color: Theme.dark ? "#372c1b" : "#fff4dc"
            implicitHeight: connectionWarning.implicitHeight + Theme.px(16)
            visible: !backend.connected || !backend.state.listening

            SLabel {
                id: connectionWarning
                anchors.centerIn: parent
                color: Theme.warn
                size: 14
                text: !backend.connected ? "Lost connection to SpearHead. Reopen the app to reconnect." : "Others can’t reach you right now: port " + backend.state.port + " is in use. Run spearhead doctor for help."
                width: parent.width - Theme.px(32)
                wrapMode: Text.Wrap
            }
        }
        RowLayout {
            Layout.fillHeight: true
            Layout.fillWidth: true
            spacing: 0

            Rectangle {
                id: sidebar
                Layout.fillHeight: true
                Layout.preferredWidth: root.narrow ? root.width : Math.max(Theme.px(304), Math.min(root.width * 0.3, Theme.px(400)))
                color: Theme.sidebar
                objectName: "sidebar"
                visible: root.showSidebar

                Rectangle {
                    anchors.right: parent.right
                    color: Theme.line
                    height: parent.height
                    width: 1
                }
                Item {
                    id: brandHeader
                    height: Theme.px(82)
                    width: parent.width

                    NativeGlyph {
                        anchors.verticalCenter: parent.verticalCenter
                        height: width
                        name: "logo"
                        objectName: "brandLogo"
                        width: Theme.px(44)
                        x: Theme.px(16)
                    }
                    Column {
                        id: brandText
                        anchors.verticalCenter: parent.verticalCenter
                        spacing: Theme.px(3)
                        width: headerActions.x - x - Theme.px(8)
                        x: Theme.px(72)

                        SLabel {
                            elide: Text.ElideRight
                            font.letterSpacing: Theme.px(-.7)
                            font.weight: Font.Bold
                            objectName: "brandTitle"
                            size: 20
                            text: "SpearHead"
                            width: parent.width
                        }
                        Row {
                            spacing: Theme.px(5)
                            width: parent.width

                            Rectangle {
                                anchors.verticalCenter: parent.verticalCenter
                                color: backend.connected && backend.state.listening ? Theme.ok : Theme.muted
                                height: width
                                radius: width / 2
                                width: Theme.px(6)
                            }
                            SLabel {
                                color: Theme.muted
                                elide: Text.ElideRight
                                objectName: "nearbyIndicator"
                                size: 11
                                text: (backend.state.nearby || []).length + " people nearby"
                                width: parent.width - Theme.px(11)
                            }
                        }
                    }
                    Row {
                        id: headerActions
                        anchors.right: parent.right
                        anchors.rightMargin: Theme.px(12)
                        anchors.verticalCenter: parent.verticalCenter
                        spacing: 0

                        IconButton {
                            height: width
                            symbol: "add"
                            tip: "Add a person"
                            width: Theme.px(32)

                            onClicked: backend.beginAdd()
                        }
                        IconButton {
                            height: width
                            objectName: "settingsButton"
                            symbol: "settings"
                            tip: "Settings"
                            width: Theme.px(32)

                            onClicked: dialogs.showSettings()
                        }
                        IconButton {
                            height: width
                            symbol: "help"
                            tip: "Help"
                            width: Theme.px(32)

                            onClicked: dialogs.showHelp()
                        }
                    }
                }
                ListView {
                    id: people

                    property ScrollEdge edgeEffect: ScrollEdge {
                        flickable: people
                    }

                    anchors.bottom: sidebarBottom.top
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.px(8)
                    anchors.right: parent.right
                    anchors.rightMargin: Theme.px(8)
                    anchors.top: brandHeader.bottom
                    clip: true
                    model: backend.peopleModel
                    objectName: "peopleList"
                    spacing: Theme.px(3)

                    ScrollBar.vertical: ScrollBar {
                        policy: ScrollBar.AsNeeded
                    }
                    delegate: PersonRow {
                        onClicked: root.choose(item.key)
                        onContextRequested: function (identity, px, py) {
                            if (identity !== "self" && identity !== "lan")
                                root.contactMenu(identity, px, py);
                        }
                    }
                    footer: Column {
                        spacing: Theme.px(12)
                        topPadding: Theme.px(26)
                        width: people.width

                        NativeGlyph {
                            anchors.horizontalCenter: parent.horizontalCenter
                            height: width
                            name: "search"
                            visible: root.noSearchResults()
                            width: Theme.px(28)
                        }
                        SLabel {
                            color: Theme.muted
                            font.weight: Font.DemiBold
                            horizontalAlignment: Text.AlignHCenter
                            size: 14
                            text: "No " + (backend.mode === "network" ? "devices" : backend.mode) + " match your search"
                            visible: root.noSearchResults()
                            width: parent.width
                            wrapMode: Text.Wrap
                        }
                        SLabel {
                            color: Theme.muted
                            horizontalAlignment: Text.AlignHCenter
                            size: 13
                            text: "Try a name, device, or address."
                            visible: root.noSearchResults()
                            width: parent.width
                        }
                        SLabel {
                            color: Theme.muted
                            horizontalAlignment: Text.AlignHCenter
                            size: 14
                            text: "No conversations yet."
                            visible: backend.mode === "conversations" && people.count === 0 && !backend.sidebarSearch
                            width: parent.width
                        }
                        SButton {
                            anchors.horizontalCenter: parent.horizontalCenter
                            kind: "primary"
                            text: "Add a person"
                            visible: backend.mode === "conversations" && people.count === 0 && !backend.sidebarSearch

                            onClicked: backend.beginAdd()
                        }
                    }
                    header: Column {
                        spacing: Theme.px(6)
                        width: people.width

                        Column {
                            visible: backend.mode === "conversations" && pendingRepeater.count > 0
                            width: parent.width

                            SLabel {
                                color: Theme.muted
                                font.letterSpacing: Theme.px(.6)
                                font.weight: Font.Bold
                                size: 12
                                text: "WANTS TO CONNECT"
                                x: Theme.px(8)
                            }
                            Repeater {
                                id: pendingRepeater
                                model: backend.mode === "conversations" ? root.filteredRequests() : []

                                delegate: Rectangle {
                                    required property var modelData

                                    border.color: Theme.line
                                    color: Theme.surface
                                    height: requestContent.implicitHeight + Theme.px(24)
                                    radius: Theme.px(16)
                                    width: people.width

                                    Column {
                                        id: requestContent
                                        spacing: Theme.px(6)
                                        width: parent.width - Theme.px(24)
                                        x: Theme.px(12)
                                        y: Theme.px(12)

                                        SLabel {
                                            font.weight: Font.Bold
                                            text: modelData.label
                                        }
                                        SLabel {
                                            color: Theme.muted
                                            size: 13
                                            text: "Identity words: " + modelData.fingerprint
                                            width: parent.width
                                            wrapMode: Text.Wrap
                                        }
                                        SLabel {
                                            color: Theme.warn
                                            size: 13
                                            text: modelData.warn || ""
                                            visible: !!modelData.warn
                                            width: parent.width
                                            wrapMode: Text.Wrap
                                        }
                                        Row {
                                            spacing: Theme.px(8)

                                            SButton {
                                                compact: true
                                                kind: "primary"
                                                text: "Accept"

                                                onClicked: backend.contactAction(modelData.pub, "accept")
                                            }
                                            SButton {
                                                compact: true
                                                text: "Decline"

                                                onClicked: backend.contactAction(modelData.pub, "decline")
                                            }
                                        }
                                    }
                                }
                            }
                        }
                        AbstractButton {
                            height: Theme.px(48)
                            objectName: "sidebarSelector"
                            width: parent.width - Theme.px(16)
                            x: Theme.px(8)

                            background: Rectangle {
                                border.color: viewMenu.opened ? Theme.primary : Theme.line
                                color: viewMenu.opened ? Theme.soft : Theme.surface
                                radius: Theme.px(11)
                            }
                            contentItem: Item {
                                NativeGlyph {
                                    anchors.verticalCenter: parent.verticalCenter
                                    color: Theme.primary
                                    height: width
                                    name: backend.mode === "conversations" ? "chat" : backend.mode
                                    width: Theme.px(20)
                                    x: Theme.px(14)
                                }
                                SLabel {
                                    anchors.verticalCenter: parent.verticalCenter
                                    font.weight: Font.DemiBold
                                    size: 15
                                    text: backend.mode.charAt(0).toUpperCase() + backend.mode.slice(1)
                                    x: Theme.px(45)
                                }
                                NativeGlyph {
                                    anchors.right: parent.right
                                    anchors.rightMargin: Theme.px(14)
                                    anchors.verticalCenter: parent.verticalCenter
                                    height: width
                                    name: "chevron"
                                    width: Theme.px(16)
                                }
                            }

                            Keys.onDownPressed: root.openViewMenu(this)
                            onClicked: viewMenu.visible ? viewMenu.close() : root.openViewMenu(this)
                        }
                        Item {
                            height: Theme.px(3)
                            width: 1
                        }
                    }
                }
                Column {
                    id: sidebarBottom
                    anchors.bottom: parent.bottom
                    width: parent.width

                    Item {
                        height: Theme.px(62)
                        width: parent.width

                        SInput {
                            anchors.left: parent.left
                            anchors.margins: Theme.px(13)
                            anchors.right: parent.right
                            anchors.verticalCenter: parent.verticalCenter
                            font.pixelSize: Theme.px(14)
                            height: Theme.px(42)
                            objectName: "sidebarSearch"
                            placeholderText: "Search " + backend.mode
                            rightPadding: Theme.px(40)
                            text: backend.sidebarSearch

                            onTextEdited: backend.sidebarSearch = text

                            IconButton {
                                anchors.right: parent.right
                                anchors.rightMargin: Theme.px(4)
                                anchors.verticalCenter: parent.verticalCenter
                                height: width
                                symbol: "close"
                                tip: "Clear sidebar search"
                                visible: backend.sidebarSearch.length > 0
                                width: Theme.px(28)

                                onClicked: {
                                    backend.sidebarSearch = "";
                                    parent.forceActiveFocus();
                                }
                            }
                        }
                    }
                    Rectangle {
                        color: Theme.sidebar
                        height: visible ? Theme.px(49) : 0
                        visible: !!backend.state.outbox || !!backend.state.failed
                        width: parent.width

                        Rectangle {
                            color: Theme.line
                            height: 1
                            width: parent.width
                        }
                        Row {
                            anchors.verticalCenter: parent.verticalCenter
                            spacing: Theme.px(9)
                            x: Theme.px(16)

                            Rectangle {
                                anchors.verticalCenter: parent.verticalCenter
                                color: backend.connected && backend.state.listening ? Theme.ok : Theme.muted
                                height: width
                                radius: width / 2
                                width: Theme.px(10)
                            }
                            SLabel {
                                color: Theme.muted
                                elide: Text.ElideRight
                                size: 14
                                text: (backend.state.outbox ? backend.state.outbox + " waiting to send" : "") + (backend.state.failed ? (backend.state.outbox ? " · " : "") + backend.state.failed + " couldn't be delivered" : "")
                                width: sidebar.width - Theme.px(50)
                            }
                        }
                    }
                }
            }
            Item {
                id: main
                Layout.fillHeight: true
                Layout.fillWidth: true
                objectName: "mainPane"
                visible: !root.narrow || root.detailShown

                Rectangle {
                    anchors.fill: parent
                    color: Theme.bg
                }
                Details {
                    anchors.fill: parent
                    narrow: root.narrow
                    visible: backend.mode !== "conversations"

                    onGoBack: root.detailShown = false
                }
                Item {
                    anchors.fill: parent
                    visible: backend.mode === "conversations" && backend.current.length === 0

                    Column {
                        anchors.centerIn: parent
                        spacing: Theme.px(14)
                        width: Math.min(parent.width - Theme.px(48), Theme.px(530))

                        NativeGlyph {
                            anchors.horizontalCenter: parent.horizontalCenter
                            color: Theme.primary
                            height: width
                            name: "chat"
                            width: Theme.px(92)
                        }
                        SLabel {
                            font.weight: Font.Bold
                            horizontalAlignment: Text.AlignHCenter
                            size: 28
                            text: (backend.state.contacts || []).length > 0 ? "Pick a conversation" : "Welcome to SpearHead"
                            width: parent.width
                        }
                        SLabel {
                            color: Theme.muted
                            horizontalAlignment: Text.AlignHCenter
                            text: "Private messages between computers on your network. No accounts, no servers."
                            width: parent.width
                            wrapMode: Text.Wrap
                        }
                        SLabel {
                            color: Theme.muted
                            size: 14
                            text: "1   Add a person nearby, or share an invite code.\n2   They accept your request.\n3   Compare identity words over a trusted channel."
                            width: parent.width
                            wrapMode: Text.Wrap
                        }
                        SButton {
                            anchors.horizontalCenter: parent.horizontalCenter
                            kind: "primary"
                            text: "Add a person"

                            onClicked: backend.beginAdd()
                        }
                    }
                }
                Item {
                    id: conversation
                    anchors.fill: parent
                    visible: backend.mode === "conversations" && backend.current.length > 0

                    Rectangle {
                        id: chatHeader
                        color: Theme.surface
                        height: Theme.px(75)
                        objectName: "conversationHeader"
                        width: parent.width

                        Rectangle {
                            anchors.bottom: parent.bottom
                            color: Theme.line
                            height: 1
                            width: parent.width
                        }
                        RowLayout {
                            anchors.fill: parent
                            anchors.leftMargin: Theme.px(16)
                            anchors.rightMargin: Theme.px(16)
                            spacing: Theme.px(12)

                            IconButton {
                                symbol: "back"
                                tip: "Back to conversations"
                                visible: root.narrow

                                onClicked: backend.back()
                            }
                            Avatar {
                                identity: backend.current
                                title: backend.contact.label || ""
                            }
                            Column {
                                Layout.fillWidth: true

                                SLabel {
                                    elide: Text.ElideRight
                                    font.weight: Font.Bold
                                    objectName: "conversationTitle"
                                    size: 18
                                    text: backend.contact.label || ""
                                    width: parent.width
                                }
                                SLabel {
                                    color: Theme.muted
                                    elide: Text.ElideRight
                                    size: 14
                                    text: backend.contact.status === "blocked" ? "Blocked" : backend.contact.nearby ? "Nearby now" : "Last seen " + backend.lastSeen(backend.contact.last_seen || 0)
                                    width: parent.width
                                }
                            }
                            Rectangle {
                                id: searchPill
                                Layout.fillWidth: true
                                Layout.maximumWidth: Theme.px(300)
                                Layout.minimumWidth: Theme.px(120)
                                Layout.preferredHeight: Theme.px(40)
                                Layout.preferredWidth: Theme.px(300)
                                border.color: searchInput.activeFocus ? Theme.primary : Theme.line
                                color: Theme.bg
                                objectName: "searchPill"
                                radius: height / 2
                                visible: backend.searchOpen

                                NativeGlyph {
                                    anchors.verticalCenter: parent.verticalCenter
                                    height: width
                                    name: "search"
                                    width: Theme.px(18)
                                    x: Theme.px(14)
                                }
                                SInput {
                                    id: searchInput
                                    anchors.fill: parent
                                    font.pixelSize: Theme.px(14)
                                    leftPadding: Theme.px(40)
                                    objectName: "messageSearch"
                                    placeholderText: "Search this conversation"
                                    rightPadding: Theme.px(14)
                                    text: backend.search

                                    background: Item {
                                    }

                                    Keys.onEscapePressed: backend.searchOpen = false
                                    onTextEdited: backend.search = text
                                }
                            }
                            IconButton {
                                id: searchToggle
                                Layout.preferredHeight: Theme.px(40)
                                Layout.preferredWidth: Theme.px(40)
                                objectName: "searchButton"
                                symbol: backend.searchOpen ? "close" : "search"
                                tip: backend.searchOpen ? "Close search" : "Search messages"

                                // Same outline as the search bar beside it.
                                background: Rectangle {
                                    border.color: searchToggle.activeFocus && searchToggle.focusReason === Qt.TabFocusReason ? Theme.muted : Theme.line
                                    color: searchToggle.hovered ? Theme.soft : Theme.bg
                                    radius: width / 2
                                }

                                onClicked: {
                                    backend.searchOpen = !backend.searchOpen;
                                    if (backend.searchOpen)
                                        searchInput.forceActiveFocus();
                                    else
                                        draftInput.forceActiveFocus();
                                }
                            }
                            SButton {
                                compact: true
                                objectName: "verifyButton"
                                text: backend.contact.verified ? "✓ Verified" : "Not verified · Verify"
                                visible: backend.contact.status === "accepted" && main.width > 576

                                background: Rectangle {
                                    border.color: backend.contact.verified ? Theme.ok : Theme.warn
                                    color: Theme.surface
                                    radius: height / 2
                                }
                                contentItem: SLabel {
                                    color: backend.contact.verified ? Theme.ok : Theme.warn
                                    font.weight: Font.DemiBold
                                    horizontalAlignment: Text.AlignHCenter
                                    size: 13
                                    text: parent.text
                                }

                                onClicked: backend.verify()
                            }
                            IconButton {
                                symbol: "more"
                                tip: "Conversation actions"

                                onClicked: {
                                    var p = mapToItem(Overlay.overlay, 0, height);
                                    chatMenu.x = Math.min(p.x, root.width - chatMenu.width - 8);
                                    chatMenu.y = p.y;
                                    chatMenu.open();
                                }
                            }
                        }
                    }
                    ListView {
                        id: chatLog

                        property ScrollEdge edgeEffect: ScrollEdge {
                            flickable: chatLog
                        }

                        anchors.bottom: composer.top
                        anchors.left: parent.left
                        anchors.margins: Theme.px(16)
                        anchors.right: parent.right
                        anchors.top: chatHeader.bottom
                        anchors.topMargin: Theme.px(24)
                        clip: true
                        model: backend.messages
                        objectName: "chatLog"
                        spacing: 0

                        ScrollBar.vertical: ScrollBar {
                            policy: ScrollBar.AsNeeded
                        }
                        delegate: MessageRow {
                            onContextRequested: function (message, px, py, selection) {
                                root.messageMenu(message, px, py, selection);
                            }
                            onSaveFile: function (identity, name) {
                                root.saveAttachment(identity, name);
                            }
                        }
                        footer: Item {
                            height: Theme.px(12)
                            width: 1
                        }

                        onAtYEndChanged: if (atYEnd)
                            root.newerMessages = false

                        SLabel {
                            anchors.horizontalCenter: parent.horizontalCenter
                            color: Theme.muted
                            horizontalAlignment: Text.AlignHCenter
                            size: 14
                            text: backend.search.trim() ? "No messages match “" + backend.search.trim() + "”." : "No messages yet. Say hello below."
                            visible: chatLog.count === 0
                            width: parent.width
                            wrapMode: Text.Wrap
                            y: Theme.px(48)
                        }
                    }
                    SButton {
                        anchors.bottom: composer.top
                        anchors.bottomMargin: Theme.px(12)
                        anchors.horizontalCenter: parent.horizontalCenter
                        compact: true
                        kind: "primary"
                        text: "New messages ↓"
                        visible: root.newerMessages

                        onClicked: {
                            chatLog.positionViewAtEnd();
                            root.newerMessages = false;
                        }
                    }
                    Rectangle {
                        id: composer
                        anchors.bottom: parent.bottom
                        color: Theme.surface
                        height: composerColumn.implicitHeight + Theme.px(28)
                        width: parent.width

                        Rectangle {
                            color: Theme.line
                            height: 1
                            width: parent.width
                        }
                        Column {
                            id: composerColumn
                            spacing: Theme.px(7)
                            width: parent.width - Theme.px(32)
                            x: Theme.px(16)
                            y: Theme.px(14)

                            SLabel {
                                color: Theme.muted
                                size: 13
                                text: backend.transferText || (backend.contact.status === "blocked" ? "You blocked this person. Unblock them to send messages." : backend.contact.status !== "accepted" ? "They need to accept your request before you can send messages." : "Read once is on: this message is deleted after they open it.")
                                visible: backend.contact.status !== "accepted" || backend.burn || !!backend.transferText
                                width: parent.width
                                wrapMode: Text.Wrap
                            }
                            RowLayout {
                                spacing: Theme.px(8)
                                width: parent.width

                                IconButton {
                                    enabled: backend.contact.status === "accepted"
                                    symbol: "attach"
                                    tip: "Attach file"

                                    onClicked: uploadDialog.open()
                                }
                                IconButton {
                                    enabled: backend.contact.status === "accepted"
                                    selected: backend.burn
                                    symbol: "burn"
                                    tip: "Read once: deleted after they open it"

                                    onClicked: backend.burn = !backend.burn
                                }
                                SScrollView {
                                    Layout.fillWidth: true
                                    Layout.preferredHeight: Math.min(Theme.px(150), Math.max(Theme.px(44), draftInput.contentHeight + Theme.px(20)))

                                    TextArea {
                                        id: draftInput
                                        bottomPadding: Theme.px(10)
                                        color: Theme.fg
                                        enabled: backend.contact.status === "accepted"
                                        font.family: Theme.family
                                        font.pixelSize: Theme.px(16)
                                        leftPadding: Theme.px(18)
                                        objectName: "composerInput"
                                        placeholderText: "Write a message"
                                        placeholderTextColor: Theme.muted
                                        rightPadding: Theme.px(16)
                                        selectByMouse: true
                                        selectedTextColor: Theme.primaryText
                                        selectionColor: Theme.primary
                                        text: backend.draft
                                        topPadding: Theme.px(10)
                                        wrapMode: TextEdit.Wrap

                                        background: Rectangle {
                                            border.color: draftInput.activeFocus ? Theme.primary : Theme.line
                                            color: Theme.bg
                                            radius: Theme.px(24)
                                        }

                                        Keys.onEnterPressed: function (event) {
                                            if (event.modifiers & Qt.ShiftModifier) {
                                                event.accepted = false;
                                                return;
                                            }
                                            if (!inputMethodComposing) {
                                                backend.draft = text;
                                                backend.send();
                                                event.accepted = true;
                                            }
                                        }
                                        Keys.onReturnPressed: function (event) {
                                            if (event.modifiers & Qt.ShiftModifier) {
                                                event.accepted = false;
                                                return;
                                            }
                                            if (!inputMethodComposing) {
                                                backend.draft = text;
                                                backend.send();
                                                event.accepted = true;
                                            }
                                        }
                                        onTextChanged: if (activeFocus)
                                            backend.draft = text
                                    }
                                }
                                IconButton {
                                    enabled: backend.contact.status === "accepted" && !backend.sending
                                    objectName: "sendButton"
                                    selected: true
                                    symbol: "send"
                                    tint: "white"
                                    tip: "Send message"

                                    background: Rectangle {
                                        color: "#0b5cad"
                                        radius: width / 2
                                    }
                                    contentItem: Item {
                                        NativeGlyph {
                                            anchors.centerIn: parent
                                            color: "white"
                                            height: width
                                            name: "send"
                                            width: Theme.px(22)
                                        }
                                    }

                                    onClicked: {
                                        backend.draft = draftInput.text;
                                        backend.send();
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    Menu {
        id: viewMenu
        closePolicy: Popup.CloseOnEscape | Popup.CloseOnPressOutsideParent
        objectName: "sidebarMenu"
        padding: Theme.px(6)
        parent: Overlay.overlay

        background: Rectangle {
            border.color: Theme.line
            color: Theme.surface
            radius: Theme.px(16)

            SoftShadow {
                anchors.fill: parent
                rounding: parent.radius
                z: -1
            }
        }

        Repeater {
            model: [{
                    "key": "conversations",
                    "title": "Conversations",
                    "subtitle": "Messages and shared files",
                    "icon": "chat"
                }, {
                    "key": "contacts",
                    "title": "Contacts",
                    "subtitle": "People and saved details",
                    "icon": "contacts"
                }, {
                    "key": "network",
                    "title": "Network",
                    "subtitle": "Devices and LAN information",
                    "icon": "network"
                }]

            delegate: MenuItem {
                required property var modelData

                height: Theme.px(64)
                objectName: "view-" + modelData.key

                background: Rectangle {
                    color: parent.highlighted ? Theme.soft : "transparent"
                    radius: Theme.px(11)
                }
                contentItem: RowLayout {
                    spacing: Theme.px(12)

                    NativeGlyph {
                        color: Theme.primary
                        height: width
                        name: modelData.icon
                        width: Theme.px(20)
                    }
                    Column {
                        Layout.fillWidth: true

                        SLabel {
                            font.weight: Font.DemiBold
                            size: 14
                            text: modelData.title
                        }
                        SLabel {
                            color: Theme.muted
                            size: 12
                            text: modelData.subtitle
                        }
                    }
                    NativeGlyph {
                        color: Theme.primary
                        height: width
                        name: "check"
                        visible: backend.mode === modelData.key
                        width: Theme.px(16)
                    }
                }

                onTriggered: backend.mode = modelData.key
            }
        }
    }
    Menu {
        id: contextMenu
        padding: Theme.px(6)
        parent: Overlay.overlay
        width: Theme.px(208)

        background: Rectangle {
            border.color: Theme.line
            color: Theme.surface
            radius: Theme.px(14)

            SoftShadow {
                anchors.fill: parent
                rounding: parent.radius
                z: -1
            }
        }

        SMenuItem {
            height: visible ? Theme.px(42) : 0
            objectName: "viewContextDetails"
            symbol: root.contextType === "device" ? "network" : "contacts"
            text: root.contextType === "device" ? "View device" : "View contact"
            visible: root.contextType === "contact" || root.contextType === "device"

            onTriggered: root.contextType === "device" ? backend.showDetail(root.contextPub) : backend.viewContact(root.contextPub)
        }
        SMenuItem {
            enabled: !root.contextMessage.hidden
            height: visible ? Theme.px(42) : 0
            symbol: "copy"
            text: root.contextMessage.file ? "Copy file name" : "Copy"
            visible: root.contextType === "message"

            onTriggered: backend.copy(root.contextSelection || (root.contextMessage.file ? root.contextMessage.file.name : root.contextMessage.body || ""))
        }
        MenuSeparator {
            height: visible ? Theme.px(8) : 0
            visible: root.contextType === "message"

            contentItem: Rectangle {
                color: Theme.line
                implicitHeight: 1
            }
        }
        SMenuItem {
            danger: true
            height: visible ? Theme.px(42) : 0
            symbol: "delete"
            text: "Delete…"
            visible: root.contextType === "message"

            onTriggered: backend.requestAction("delete", root.contextPub, root.contextMessage.id)
        }
    }
    Menu {
        id: chatMenu
        padding: Theme.px(6)
        parent: Overlay.overlay
        width: Theme.px(240)

        background: Rectangle {
            border.color: Theme.line
            color: Theme.surface
            radius: Theme.px(14)
        }

        SMenuItem {
            symbol: "contacts"
            text: "View contact"

            onTriggered: backend.viewContact(backend.current)
        }
        SMenuItem {
            height: visible ? Theme.px(42) : 0
            symbol: "check"
            text: "Verify identity"
            visible: backend.contact.status === "accepted"

            onTriggered: backend.verify()
        }
        SMenuItem {
            text: "Rename"

            onTriggered: dialogs.showRename()
        }
        SMenuItem {
            text: backend.contact.status === "blocked" ? "Unblock" : "Block"

            onTriggered: backend.contact.status === "blocked" ? backend.contactAction(backend.current, "unblock") : backend.requestAction("block")
        }
        SMenuItem {
            danger: true
            symbol: "delete"
            text: "Remove contact…"

            onTriggered: backend.requestAction("remove")
        }
    }
    Dialogs {
        id: dialogs
    }
    SystemDialogs.FileDialog {
        id: uploadDialog
        fileMode: SystemDialogs.FileDialog.OpenFile
        title: "Share a file"

        onAccepted: backend.upload(selectedFile)
    }
    SystemDialogs.FileDialog {
        id: downloadDialog
        fileMode: SystemDialogs.FileDialog.SaveFile
        title: "Save shared file"

        onAccepted: backend.download(root.downloadId, selectedFile)
    }
    Rectangle {
        anchors.bottom: parent.bottom
        anchors.bottomMargin: Theme.px(90)
        anchors.horizontalCenter: parent.horizontalCenter
        color: root.toastError ? "#a4161a" : "#17213a"
        height: toastLabel.implicitHeight + Theme.px(24)
        radius: Theme.px(16)
        visible: !!root.toastText
        width: Math.min(Theme.px(450), parent.width - Theme.px(32))
        z: 1000

        SLabel {
            id: toastLabel
            anchors.centerIn: parent
            color: "white"
            size: 14
            text: root.toastText
            width: parent.width - Theme.px(28)
            wrapMode: Text.Wrap
        }
    }
    Timer {
        id: toastTimer
        interval: root.toastError ? 6000 : 3500

        onTriggered: root.toastText = ""
    }
}
