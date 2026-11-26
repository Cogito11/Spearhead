#include "bridge.h"
#include "nativeicon.h"
#include "tests/fixture.h"
#include <QApplication>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickItem>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QtTest>
class DesktopTests : public QObject {
  Q_OBJECT
  QTemporaryDir preferences;
  QString output;
  QJsonObject metrics;
  struct View {
    Fixture fixture;
    Bridge bridge;
    QQmlApplicationEngine engine;
    QQuickWindow *window = nullptr;
    QStringList warnings;
    View(QJsonObject session = {})
        : bridge({{"url", fixture.url().toString()}, {"session", session}}) {
      QObject::connect(&engine, &QQmlEngine::warnings, &engine,
                       [this](const QList<QQmlError> &errors) {
                         for (const auto &e : errors)
                           warnings << e.toString();
                       });
      engine.rootContext()->setContextProperty("backend", &bridge);
      engine.rootContext()->setContextProperty("initialWidth", 1120);
      engine.rootContext()->setContextProperty("initialHeight", 760);
      engine.rootContext()->setContextProperty("runtimeCheck", false);
      engine.load(QUrl("qrc:/ui/qml/Main.qml"));
      if (!engine.rootObjects().isEmpty())
        window = qobject_cast<QQuickWindow *>(engine.rootObjects().first());
      bridge.start();
    }
    ~View() {
      if (window)
        window->hide();
    }
    QObject *find(const QString &name) {
      if (!window)
        return nullptr;
      if (window->objectName() == name)
        return window;
      auto object = window->findChild<QObject *>(name);
      if (object)
        return object;
      return visual(window->contentItem(), name);
    }
    static QQuickItem *visual(QQuickItem *item, const QString &name) {
      if (item->objectName() == name)
        return item;
      for (auto child : item->childItems())
        if (auto found = visual(child, name))
          return found;
      return nullptr;
    }
    void click(const QString &name) {
      auto item = qobject_cast<QQuickItem *>(find(name));
      if (item)
        QTest::mouseClick(
            window, Qt::LeftButton, Qt::NoModifier,
            item->mapToScene(QPointF(item->width() / 2, item->height() / 2)).toPoint());
    }
  };
  void snapshot(View &view, const QString &name) {
    QTest::qWait(200);
    auto image = view.window->grabWindow().convertToFormat(QImage::Format_RGBA8888);
    QVERIFY(!image.isNull());
    QVERIFY(image.save(output + "/" + name + ".png"));
    QJsonObject geometry;
    for (auto objectName : {"brandLogo", "brandTitle", "nearbyIndicator", "sidebarSelector",
                            "conversationHeader", "conversationTitle", "searchPill",
                            "searchButton", "sendButton", "contactLabel", "networkScan"})
      if (auto item = qobject_cast<QQuickItem *>(view.find(objectName))) {
        if (!item->isVisible())
          continue;
        auto point = item->mapToScene(QPointF(0, 0));
        geometry[objectName] = QJsonObject{{"x", point.x()},
                                           {"y", point.y()},
                                           {"width", item->width()},
                                           {"height", item->height()}};
      }
    metrics[name] = QJsonObject{
        {"viewport", QJsonArray{image.width(), image.height()}},
        {"geometry", geometry},
        {"pixels_sha256",
         QString::fromLatin1(
             QCryptographicHash::hash(
                 QByteArray(reinterpret_cast<const char *>(image.constBits()), image.sizeInBytes()),
                 QCryptographicHash::Sha256)
                 .toHex())}};
    QFile metadata(output + "/metrics.json");
    QVERIFY(metadata.open(QIODevice::WriteOnly));
    metadata.write(
        QJsonDocument(QJsonObject{{"font", QString("Noto Sans")}, {"scenes", metrics}}).toJson());
  }
private slots:
  void initTestCase() {
    if (!qEnvironmentVariable("SPEARHEAD_APPDIR").isEmpty()) {
      QFile maps("/proc/self/maps");
      QVERIFY(maps.open(QIODevice::ReadOnly));
      auto loaded = maps.readAll();
      for (const QByteArray &name :
           {QByteArray("libQt6Core"), QByteArray("libQt6Gui"), QByteArray("libQt6Qml"),
            QByteArray("libQt6Quick"), QByteArray("libQt6Widgets"), QByteArray("libQt6Network"),
            QByteArray("libQt6Svg")}) {
        bool found = false;
        for (auto line : loaded.split('\n'))
          if (line.contains(name)) {
            found = true;
            QVERIFY2(line.contains(qEnvironmentVariable("SPEARHEAD_APPDIR").toUtf8() + "/usr/lib/"),
                     line.constData());
          }
        QVERIFY(found);
      }
    }
    qmlRegisterType<NativeIcon>("SpearHead", 1, 0, "NativeIcon");
    qmlRegisterType<NativeBubble>("SpearHead", 1, 0, "NativeBubble");
    QCoreApplication::setApplicationName("SpearHeadTests");
    QCoreApplication::setOrganizationName("SpearHeadTests");
    QSettings::setDefaultFormat(QSettings::IniFormat);
    QSettings::setPath(QSettings::IniFormat, QSettings::UserScope, preferences.path());
    output = qEnvironmentVariable("SPEARHEAD_TEST_OUTPUT",
                                  QDir::currentPath() + "/.build/reports/native");
    QDir().mkpath(output);
  }
  void completeWindowAndReferenceScenes() {
    View view({{"theme", "light"}, {"fontSize", 100}});
    QVERIFY(view.window);
    QTRY_COMPARE(view.bridge.state().value("me").toMap().value("name").toString(), QString("Alex"));
    view.bridge.select("bob-key");
    QTRY_COMPARE(view.bridge.messages()->rowCount(), 4);
    for (auto theme : {QString("light"), QString("dark")}) {
      view.bridge.setTheme(theme);
      snapshot(view, "chat-" + theme);
      view.click("sidebarSelector");
      snapshot(view, "picker-" + theme);
      view.window->requestActivate();
      QTest::keyClick(view.window, Qt::Key_Escape);
      view.bridge.setSidebarSearch("no-such-contact");
      snapshot(view, "empty-search-" + theme);
      view.bridge.setSidebarSearch("");
      view.bridge.setSearchOpen(true);
      view.bridge.setSearch("Thanks");
      QTRY_COMPARE(view.bridge.messages()->rowCount(), 2);
      snapshot(view, "search-" + theme);
      view.bridge.setSearchOpen(false);
      QTRY_VERIFY(view.find("message-hello"));
      auto message = qobject_cast<QQuickItem *>(view.find("message-hello"));
      QVERIFY(message);
      QTest::mouseClick(view.window, Qt::RightButton, Qt::NoModifier,
                        message->mapToScene(QPointF(30, 20)).toPoint());
      snapshot(view, "context-" + theme);
      QTest::keyClick(view.window, Qt::Key_Escape);
      view.click("settingsButton");
      QTest::qWait(30);
      auto save = view.find("settingsSave");
      QVERIFY(save);
      auto label = save->findChild<QObject *>("buttonLabel");
      QVERIFY(label);
      QCOMPARE(label->property("color").value<QColor>(),
               theme == "light" ? QColor("#ffffff") : QColor("#0d1b2f"));
      snapshot(view, "settings-" + theme);
      for (int scale : {110, 140, 160}) {
        view.bridge.setFontSize(scale);
        QTest::qWait(30);
        auto scroll = view.find("settingsScroll");
        QVERIFY(scroll);
        auto viewport = scroll->property("contentItem").value<QQuickItem *>();
        auto dark = qobject_cast<QQuickItem *>(view.find("theme-dark"));
        QVERIFY(viewport);
        QVERIFY(dark);
        auto right = dark->mapToItem(viewport, QPointF(dark->width(), 0)).x();
        QVERIFY2(viewport->width() - right >= scale / 100.0,
                 "The theme button must keep a gutter inside the clipped viewport");
        snapshot(view, "settings-" + theme + "-" + QString::number(scale));
      }
      view.bridge.setFontSize(100);
      auto settings = view.find("settingsDialog");
      QVERIFY(settings);
      QMetaObject::invokeMethod(settings, "close");
      view.bridge.viewContact("bob-key");
      snapshot(view, "contacts-" + theme);
      view.bridge.setMode("network");
      QTRY_COMPARE(view.bridge.network().value("hostname").toString(), QString("Alex laptop"));
      snapshot(view, "network-" + theme);
      view.bridge.select("bob-key");
      view.window->resize(520, 700);
      snapshot(view, "narrow-" + theme);
      view.bridge.setSearchOpen(true);
      view.bridge.setSearch("Thanks");
      snapshot(view, "narrow-search-" + theme);
      view.bridge.setSearchOpen(false);
      view.window->resize(1120, 760);
      view.bridge.setFontSize(140);
      snapshot(view, "scaled-" + theme);
      view.bridge.setSearchOpen(true);
      view.bridge.setSearch("Thanks");
      snapshot(view, "scaled-search-" + theme);
      view.bridge.setSearchOpen(false);
      view.bridge.setFontSize(100);
    }
    QVERIFY2(view.warnings.isEmpty(), qPrintable(view.warnings.join("\n")));
  }
  void panelTogglesAndClosePreference() {
    View view;
    QVERIFY(view.window);
    QTRY_VERIFY(!view.bridge.people().isEmpty());
    view.bridge.select("bob-key");
    QTRY_COMPARE(view.bridge.messages()->rowCount(), 4);
    view.bridge.setDraft("Keep my draft");
    auto searchButton = view.find("searchButton");
    auto searchPill = qobject_cast<QQuickItem *>(view.find("searchPill"));
    auto messageSearch = qobject_cast<QQuickItem *>(view.find("messageSearch"));
    QVERIFY(searchButton && searchPill && messageSearch);
    QVERIFY(!searchPill->isVisible());
    QCOMPARE(searchButton->property("symbol").toString(), QString("search"));
    view.click("searchButton");
    QTRY_VERIFY(view.bridge.searchOpen());
    // The bar opens in the top bar, left of the button, which becomes a close button.
    QTRY_VERIFY(searchPill->isVisible());
    QVERIFY(messageSearch->hasActiveFocus());
    QCOMPARE(searchButton->property("symbol").toString(), QString("close"));
    auto buttonItem = qobject_cast<QQuickItem *>(searchButton);
    QVERIFY(searchPill->mapToScene(QPointF(searchPill->width(), 0)).x() <=
            buttonItem->mapToScene(QPointF(0, 0)).x());
    view.bridge.setSearch("Thanks");
    QCOMPARE(view.bridge.messages()->rowCount(), 2);
    view.click("searchButton");
    QTRY_VERIFY(!view.bridge.searchOpen());
    QTRY_VERIFY(!searchPill->isVisible());
    QCOMPARE(searchButton->property("symbol").toString(), QString("search"));
    QCOMPARE(view.bridge.search(), QString());
    QCOMPARE(view.bridge.messages()->rowCount(), 4);
    QCOMPARE(view.bridge.draft(), QString("Keep my draft"));
    QSignalSpy changes(&view.bridge, &Bridge::messagesChanged);
    view.click("searchButton");
    QTRY_VERIFY(view.bridge.searchOpen());
    view.click("searchButton");
    QTRY_VERIFY(!view.bridge.searchOpen());
    QCOMPARE(changes.size(), 0);

    auto menu = view.find("sidebarMenu");
    QVERIFY(menu);
    for (int i = 0; i < 3; i++) {
      view.click("sidebarSelector");
      QTRY_VERIFY(menu->property("visible").toBool());
      view.click("sidebarSelector");
      QTRY_VERIFY(!menu->property("visible").toBool());
    }
    auto selector = qobject_cast<QQuickItem *>(view.find("sidebarSelector"));
    QVERIFY(selector);
    selector->forceActiveFocus();
    QTest::keyClick(view.window, Qt::Key_Down);
    QTRY_VERIFY(menu->property("visible").toBool());
    QCOMPARE(menu->property("width").toReal(), selector->width());
    QCOMPARE(menu->property("x").toReal(), 0.0);
    QVERIFY(menu->property("y").toReal() > selector->height());
    QTest::keyClick(view.window, Qt::Key_Escape);
    QTRY_VERIFY(!menu->property("visible").toBool());
    view.click("sidebarSelector");
    QTRY_VERIFY(menu->property("visible").toBool());
    QTest::mouseClick(view.window, Qt::LeftButton, Qt::NoModifier, QPoint(900, 400));
    QTRY_VERIFY(!menu->property("visible").toBool());

    QVERIFY(view.bridge.closeToTray());
    view.click("settingsButton");
    auto checkbox = qobject_cast<QQuickItem *>(view.find("closeToTray"));
    QVERIFY(checkbox);
    QVERIFY(checkbox->property("checked").toBool());
    // The final settings fields may be below the fold in shorter windows.
    auto scroll = view.find("settingsScroll");
    QVERIFY(scroll);
    auto viewport = scroll->property("contentItem").value<QQuickItem *>();
    QVERIFY(viewport);
    viewport->setProperty("contentY", viewport->property("originY").toReal() +
                                          viewport->property("contentHeight").toReal() -
                                          viewport->height());
    QTest::qWait(30);
    view.click("closeToTray");
    QTRY_VERIFY(!view.bridge.closeToTray());
    QVERIFY(!view.bridge.session(1120, 760).value("closeToTray").toBool(true));
    auto settings = view.find("settingsDialog");
    QVERIFY(settings);
    QMetaObject::invokeMethod(settings, "close");
    View reopened(view.bridge.session(1120, 760));
    QVERIFY(!reopened.bridge.closeToTray());
    reopened.window->hide();
    reopened.bridge.setCloseToTray(true);
    QVERIFY(reopened.bridge.session(1120, 760).value("closeToTray").toBool());

    view.bridge.viewContact("bob-key");
    view.fixture.holdSave = true;
    view.bridge.setEditLabel("Saving contact");
    view.bridge.saveContact();
    QTRY_VERIFY(view.fixture.heldSave);
    QVERIFY(!view.bridge.closeWindow(1120, 760));
    view.fixture.releaseSave();
    QTRY_VERIFY(!view.bridge.saving());
    QVERIFY2(view.warnings.isEmpty(), qPrintable(view.warnings.join("\n")));
  }
  void identitySafetyContactsAndSettings() {
    View view;
    QVERIFY(view.window);
    QTRY_VERIFY(!view.bridge.people().isEmpty());
    view.bridge.select("bob-key");
    QTRY_VERIFY(view.bridge.messages()->rowCount() > 0);
    QSignalSpy dialog(&view.bridge, &Bridge::openDialog);
    view.bridge.verify();
    QTRY_COMPARE(dialog.size(), 1);
    QCOMPARE(view.bridge.safetyCode(), QString("alpha beta gamma delta epsilon zeta eta theta"));
    view.bridge.contactAction("bob-key", "verify");
    QTRY_VERIFY(!view.fixture.posts.isEmpty());
    view.bridge.lookup("rkt1:test");
    QTRY_COMPARE(view.bridge.found().value("pub").toString(), QString("new-key"));
    view.bridge.addFound("New friend");
    QTRY_COMPARE(view.bridge.current(), QString("new-key"));
    view.bridge.contactAction("new-key", "block");
    QTRY_COMPARE(view.bridge.contact().value("status").toString(), QString("blocked"));
    view.bridge.contactAction("new-key", "unblock");
    QTRY_COMPARE(view.bridge.contact().value("status").toString(), QString("accepted"));
    view.bridge.saveSettings("Alex renamed", 30, false);
    QTRY_VERIFY(!view.bridge.saving());
    QTRY_COMPARE(view.bridge.state().value("me").toMap().value("name").toString(),
                 QString("Alex renamed"));
    QCOMPARE(view.bridge.state().value("settings").toMap().value("retention_days").toInt(), 30);
    view.bridge.setTheme("dark");
    view.bridge.setFontSize(120);
    QCOMPARE(view.bridge.theme(), QString("dark"));
    QCOMPARE(view.bridge.fontSize(), 120);
    view.bridge.copy("Copied message");
    QCOMPARE(QApplication::clipboard()->text(), QString("Copied message"));
    QVERIFY2(view.warnings.isEmpty(), qPrintable(view.warnings.join("\n")));
  }
  void sendRaceSecretConfirmationAndDrafts() {
    View view;
    QTRY_VERIFY(!view.bridge.people().isEmpty());
    auto contacts = view.fixture.state.value("contacts").toArray();
    auto carol = contacts[0].toObject();
    carol["pub"] = "carol-key";
    carol["label"] = "Carol";
    contacts << carol;
    view.fixture.state["contacts"] = contacts;
    view.bridge.refresh();
    QTRY_COMPARE(view.bridge.people().size(), 2);
    view.bridge.select("bob-key");
    view.bridge.setDraft("Original Bob message");
    view.fixture.sendDelay = 150;
    view.bridge.send();
    view.bridge.send();
    view.bridge.select("carol-key");
    view.bridge.setDraft("Carol draft");
    QTRY_VERIFY(!view.bridge.sending());
    QCOMPARE(view.bridge.draft(), QString("Carol draft"));
    int sends = 0;
    for (auto p : view.fixture.posts)
      if (p.first == "/api/send") {
        sends++;
        QCOMPARE(p.second.value("c").toString(), QString("bob-key"));
      }
    QCOMPARE(sends, 1);
    view.bridge.select("bob-key");
    view.bridge.setDraft("password: test-sensitive-text");
    QSignalSpy secrets(&view.bridge, &Bridge::secrets);
    view.bridge.send();
    QTRY_COMPARE(secrets.size(), 1);
    view.bridge.select("carol-key");
    view.bridge.setDraft("New Carol draft");
    view.bridge.confirmSend(true);
    QTRY_VERIFY(!view.bridge.sending());
    QCOMPARE(view.bridge.draft(), QString("New Carol draft"));
    QCOMPARE(view.fixture.posts.last().second.value("c").toString(), QString("bob-key"));
    QVERIFY(view.fixture.posts.last().second.value("force").toBool());
    auto session = view.bridge.session(1120, 760);
    QVERIFY(!session.contains("messages"));
    QVERIFY(session.value("drafts").toArray().size() >= 2);
    QVERIFY(!view.bridge.richText("<script>alert(1)</script> https://example.com/?a=1&b=2")
                 .contains("<script>"));
    QVERIFY(view.bridge.richText("file:///etc/passwd").indexOf("href") < 0);
  }
  void pendingContactSaveAndLatestNetworkResponse() {
    View view;
    QTRY_VERIFY(!view.bridge.people().isEmpty());
    view.bridge.viewContact("bob-key");
    view.bridge.setEditLabel("First saved label");
    view.fixture.holdSave = true;
    view.bridge.saveContact();
    QTRY_VERIFY(view.fixture.heldSave);
    view.bridge.setEditLabel("New unsaved label");
    view.fixture.releaseSave();
    QTRY_VERIFY(!view.bridge.saving());
    QCOMPARE(view.bridge.editLabel(), QString("New unsaved label"));
    QVERIFY(view.bridge.editing());
    view.bridge.saveContact();
    QTRY_VERIFY(view.fixture.heldSave);
    view.bridge.showDetail("self");
    view.bridge.setEditLabel("My unsaved name");
    view.fixture.releaseSave();
    QTRY_VERIFY(!view.bridge.saving());
    QCOMPARE(view.bridge.editLabel(), QString("My unsaved name"));
    QVERIFY(view.bridge.editing());
    auto older = view.fixture.network;
    older["hostname"] = "Old inventory";
    auto newer = view.fixture.network;
    newer["hostname"] = "Latest inventory";
    view.fixture.networkReplies = {{180, older}, {5, newer}};
    view.bridge.setMode("network");
    QTest::qWait(30);
    view.bridge.setMode("network");
    QTRY_COMPARE(view.bridge.network().value("hostname").toString(), QString("Latest inventory"));
    QTest::qWait(220);
    QCOMPARE(view.bridge.network().value("hostname").toString(), QString("Latest inventory"));
    view.bridge.setSidebarSearch("no-match");
    QCOMPARE(view.bridge.people().size(), 1);
    view.bridge.select("bob-key");
    QCOMPARE(view.bridge.sidebarSearch(), QString());
  }
  void subtleScrollAndDetailNavigation() {
    View view;
    QVERIFY(view.window);
    QTRY_VERIFY(!view.bridge.people().isEmpty());
    view.window->resize(520, 700);
    view.bridge.viewContact("bob-key");
    QTRY_VERIFY(view.window->property("detailShown").toBool());
    auto contacts = view.fixture.state.value("contacts").toArray();
    auto contact = contacts[0].toObject();
    contact["label"] = "Updated Bobby";
    contacts[0] = contact;
    view.fixture.state["contacts"] = contacts;
    view.bridge.refresh();
    QTRY_COMPARE(
        view.bridge.state().value("contacts").toList()[0].toMap().value("label").toString(),
        QString("Updated Bobby"));
    QVERIFY(view.window->property("detailShown").toBool());
    View restored(view.bridge.session(520, 700));
    QTRY_COMPARE(restored.bridge.detail(), QString("bob-key"));
    QTRY_VERIFY(restored.window->property("detailShown").toBool());
    restored.window->hide();

    QJsonArray devices;
    for (int i = 0; i < 24; i++)
      devices << QJsonObject{{"id", "mac-device-" + QString::number(i)},
                             {"name", "LAN device " + QString::number(i)},
                             {"ip", "192.168.1." + QString::number(i + 10)}};
    view.fixture.network["devices"] = devices;
    view.bridge.setMode("network");
    QTRY_COMPARE(view.bridge.people().size(), 25);
    QVERIFY(!view.window->property("detailShown").toBool());
    QVERIFY(QMetaObject::invokeMethod(view.window, "contactMenu",
                                      Q_ARG(QVariant, QString("mac-device-0")),
                                      Q_ARG(QVariant, 100), Q_ARG(QVariant, 200)));
    QTest::qWait(30);
    auto action = view.find("viewContextDetails");
    QVERIFY(action);
    QCOMPARE(action->property("text").toString(), QString("View device"));
    view.click("viewContextDetails");
    QTRY_COMPARE(view.bridge.detail(), QString("mac-device-0"));
    QCOMPARE(view.bridge.mode(), QString("network"));
    QVERIFY(view.window->property("detailShown").toBool());

    view.bridge.setMode("network");
    view.window->requestActivate();
    auto list = qobject_cast<QQuickItem *>(view.find("peopleList"));
    QVERIFY(list);
    auto effect = list->property("edgeEffect").value<QObject *>();
    QVERIFY(effect);
    QTRY_VERIFY(list->property("contentHeight").toReal() > list->height());
    QVERIFY(QMetaObject::invokeMethod(list, "positionViewAtBeginning"));
    QTest::qWait(50);
    auto touchpad = QTest::createTouchDevice(QInputDevice::DeviceType::TouchPad,
                                             QInputDevice::Capability::Position |
                                                 QInputDevice::Capability::PixelScroll |
                                                 QInputDevice::Capability::Scroll);
    auto exerciseWheel = [&](bool bottom, bool pixel) {
      auto point = list->mapToScene(QPointF(50, 100));
      auto global = view.window->mapToGlobal(point.toPoint());
      auto device = pixel ? touchpad : QPointingDevice::primaryPointingDevice();
      int direction = bottom ? -1 : 1;
      if (pixel) {
        QWheelEvent begin(point, global, QPoint(), QPoint(), Qt::NoButton, Qt::NoModifier,
                          Qt::ScrollBegin, false, Qt::MouseEventNotSynthesized, device);
        QCoreApplication::sendEvent(view.window, &begin);
      }
      QWheelEvent wheel(point, global, pixel ? QPoint(0, direction * 100) : QPoint(),
                        QPoint(0, direction * 120), Qt::NoButton, Qt::NoModifier,
                        pixel ? Qt::ScrollUpdate : Qt::NoScrollPhase, false,
                        Qt::MouseEventNotSynthesized, device);
      QCoreApplication::sendEvent(view.window, &wheel);
      QTest::qWait(30);
      qreal offset = effect->property("offset").toReal();
      QVERIFY(qAbs(offset) > 2);
      QVERIFY(qAbs(offset) <= 16.01);
      QVERIFY(bottom ? offset < 0 : offset > 0);
      if (pixel) {
        QWheelEvent end(point, global, QPoint(), QPoint(), Qt::NoButton, Qt::NoModifier,
                        Qt::ScrollEnd, false, Qt::MouseEventNotSynthesized, device);
        QCoreApplication::sendEvent(view.window, &end);
      }
      QTRY_VERIFY(qAbs(effect->property("offset").toReal()) < 0.01);
      auto shift = effect->property("displacement").value<QObject *>();
      QVERIFY(shift);
      QTRY_VERIFY(qAbs(shift->property("y").toReal()) < 0.1);
    };
    exerciseWheel(false, false);
    exerciseWheel(false, true);
    auto touch = QTest::createTouchDevice();
    bool touchInput = false;
    auto exerciseEdge = [&](bool bottom) {
      auto origin = list->mapToScene(QPointF(list->width() / 2, list->height() / 2));
      if (touchInput)
        QTest::touchEvent(view.window, touch).press(0, origin.toPoint(), view.window);
      else
        QTest::mousePress(view.window, Qt::LeftButton, Qt::NoModifier, origin.toPoint());
      bool sawFeedback = false;
      for (int step = 1; step <= 8; step++) {
        auto point = (origin + QPointF(0, (bottom ? -1 : 1) * step * 20)).toPoint();
        if (touchInput) {
          QTest::touchEvent(view.window, touch).move(0, point, view.window);
          QTest::qWait(15);
        } else
          QTest::mouseMove(view.window, point, 15);
        qreal offset = effect->property("offset").toReal();
        sawFeedback |= qAbs(offset) > 0.1;
        QVERIFY(qAbs(offset) <= 16.01);
        QVERIFY(bottom ? offset <= 0 : offset >= 0);
      }
      auto end = (origin + QPointF(0, bottom ? -160 : 160)).toPoint();
      if (touchInput)
        QTest::touchEvent(view.window, touch).release(0, end, view.window);
      else
        QTest::mouseRelease(view.window, Qt::LeftButton, Qt::NoModifier, end);
      QVERIFY(sawFeedback);
      QTRY_VERIFY(qAbs(effect->property("offset").toReal()) < 0.01);
      auto shift = effect->property("displacement").value<QObject *>();
      QVERIFY(shift);
      QTRY_VERIFY(qAbs(shift->property("y").toReal()) < 0.1);
    };
    exerciseEdge(false);
    QVERIFY(QMetaObject::invokeMethod(list, "positionViewAtEnd"));
    QTest::qWait(50);
    exerciseEdge(true);
    exerciseWheel(true, false);
    exerciseWheel(true, true);
    // Ordinary scrolling must still move the underlying viewport between edges.
    list->setProperty("contentY", list->property("originY").toReal() + 200);
    QTest::qWait(30);
    qreal before = list->property("contentY").toReal();
    auto middle = list->mapToScene(QPointF(50, 100));
    QWheelEvent normal(middle, view.window->mapToGlobal(middle.toPoint()), QPoint(),
                       QPoint(0, -120), Qt::NoButton, Qt::NoModifier, Qt::NoScrollPhase, false);
    QCoreApplication::sendEvent(view.window, &normal);
    QTRY_VERIFY(list->property("contentY").toReal() > before);
    view.bridge.setMode("conversations");
    view.window->resize(1120, 500);
    view.bridge.openDialog("help");
    QTest::qWait(50);
    auto scroll = view.find("helpScroll");
    QVERIFY(scroll);
    list = scroll->property("contentItem").value<QQuickItem *>();
    effect = scroll->property("edgeEffect").value<QObject *>();
    QVERIFY(list);
    QVERIFY(effect);
    QTRY_VERIFY(list->property("contentHeight").toReal() > list->height());
    list->setProperty("contentY", list->property("originY"));
    QTest::qWait(50);
    exerciseWheel(false, false);
    exerciseWheel(false, true);
    touchInput = true;
    exerciseEdge(false);
    list->setProperty("contentY", list->property("originY").toReal() +
                                      list->property("contentHeight").toReal() - list->height());
    QTest::qWait(50);
    exerciseEdge(true);
    touchInput = false;
    exerciseWheel(true, false);
    exerciseWheel(true, true);
    auto help = view.find("helpDialog");
    QVERIFY(help);
    QMetaObject::invokeMethod(help, "close");
    view.window->resize(1120, 760);
    view.bridge.select("bob-key");
    QTRY_COMPARE(view.bridge.messages()->rowCount(), 4);
    list = qobject_cast<QQuickItem *>(view.find("chatLog"));
    QVERIFY(list);
    effect = list->property("edgeEffect").value<QObject *>();
    QVERIFY(effect);
    QTRY_VERIFY(list->property("contentHeight").toReal() < list->height());
    exerciseWheel(false, false);
    exerciseWheel(true, false);
    QVERIFY2(view.warnings.isEmpty(), qPrintable(view.warnings.join("\n")));
  }
  void readOnceDeletionAndSessionPrivacy() {
    View view;
    auto messages = view.fixture.messages;
    messages << QJsonObject{{"id", "burn-id"},  {"body", ""},  {"hidden", true},
                            {"ts", 1700000050}, {"dir", "in"}, {"state", "delivered"}};
    view.fixture.messages = messages;
    QTRY_VERIFY(!view.bridge.people().isEmpty());
    view.bridge.select("bob-key");
    QTRY_COMPARE(view.bridge.messages()->rowCount(), 5);
    view.bridge.setSearch("one-time-secret");
    QCOMPARE(view.bridge.messages()->rowCount(), 0);
    view.bridge.setSearch("");
    view.bridge.reveal("burn-id");
    QTRY_VERIFY(
        std::any_of(view.bridge.messages()->rows.begin(), view.bridge.messages()->rows.end(),
                    [](auto row) { return row.value("body").toString() == "one-time-secret"; }));
    auto session = QJsonDocument(view.bridge.session(1120, 760)).toJson();
    QVERIFY(!session.contains("one-time-secret"));
    view.bridge.back();
    view.bridge.select("bob-key");
    QTRY_COMPARE(view.bridge.messages()->rowCount(), 4);
    QVERIFY(!QJsonDocument(
                 QJsonArray::fromVariantList(QVariantList{view.bridge.messages()->rows.last()}))
                 .toJson()
                 .contains("one-time-secret"));
    view.bridge.deleteMessage("hello");
    view.bridge.confirmAction(true);
    QTRY_COMPARE(view.bridge.messages()->rowCount(), 3);
    QVERIFY(std::none_of(view.bridge.messages()->rows.begin(), view.bridge.messages()->rows.end(),
                         [](auto row) { return row.value("id") == "hello"; }));
  }
  void streamedFilesAndIntegrity() {
    View view;
    QTRY_VERIFY(!view.bridge.people().isEmpty());
    view.bridge.select("bob-key");
    QTRY_COMPARE(view.bridge.messages()->rowCount(), 4);
    QTemporaryDir temporary;
    QString input = temporary.filePath("source.bin");
    QFile f(input);
    QVERIFY(f.open(QIODevice::WriteOnly));
    f.write(view.fixture.file);
    f.close();
    view.bridge.upload(QUrl::fromLocalFile(input));
    QTRY_VERIFY(!view.bridge.busy());
    QCOMPARE(view.fixture.uploaded, view.fixture.file);
    QString output = temporary.filePath("download.bin");
    view.bridge.download("file-id", QUrl::fromLocalFile(output));
    QTRY_VERIFY(!view.bridge.busy());
    QFile saved(output);
    QVERIFY(saved.open(QIODevice::ReadOnly));
    QCOMPARE(saved.readAll(), view.fixture.file);
    saved.close();
    saved.open(QIODevice::WriteOnly);
    saved.write("Keep my existing file");
    saved.close();
    view.fixture.file.append('x');
    view.bridge.download("file-id", QUrl::fromLocalFile(output));
    QTRY_VERIFY(!view.bridge.busy());
    saved.open(QIODevice::ReadOnly);
    QCOMPARE(saved.readAll(), QByteArray("Keep my existing file"));
    QVERIFY(
        view.bridge.suggestedFile("../unsafe-name.txt").toLocalFile().endsWith("/unsafe-name.txt"));
  }
  void stableLogoAndReopenSession() {
    View view;
    QTRY_VERIFY(!view.bridge.people().isEmpty());
    for (int i = 0; i < 60; i++) {
      view.bridge.setTheme(QStringList{"light", "dark", "system"}[i % 3]);
      view.bridge.setFontSize(i % 2 ? 100 : 140);
      view.bridge.setMode(QStringList{"contacts", "network", "conversations"}[(i / 3) % 3]);
      if (view.bridge.mode() == "conversations")
        view.bridge.select("bob-key");
      view.window->resize(i % 2 ? 1120 : 980, 760);
      QTest::qWait(15);
      auto image = view.window->grabWindow();
      auto logo = qobject_cast<QQuickItem *>(view.find("brandLogo"));
      QVERIFY(logo);
      auto point = logo->mapToScene(QPointF(logo->width() * .5, logo->height() * .38)).toPoint();
      QColor pixel = image.pixelColor(point);
      QVERIFY2(qAbs(pixel.red() - 192) < 10 && qAbs(pixel.green() - 192) < 10 &&
                   qAbs(pixel.blue() - 192) < 10,
               qPrintable(pixel.name()));
    }
    view.bridge.select("bob-key");
    view.bridge.setDraft("Unsent RAM draft");
    view.bridge.setBurn(true);
    auto session = view.bridge.session(1120, 760);
    View reopened(session);
    QTRY_COMPARE(reopened.bridge.current(), QString("bob-key"));
    QCOMPARE(reopened.bridge.draft(), QString("Unsent RAM draft"));
    QVERIFY(reopened.bridge.burn());
  }
};
int main(int argc, char **argv) {
  QQuickWindow::setSceneGraphBackend("software");
  QQuickStyle::setStyle("Basic");
  QApplication app(argc, argv);
  app.setApplicationVersion(SPEARHEAD_VERSION);
  QFont font("Noto Sans");
  font.setPixelSize(16);
  app.setFont(font);
  DesktopTests tests;
  return QTest::qExec(&tests, argc, argv);
}
#include "desktop_test.moc"
