// Native Qt 6/QML desktop with a disposable window process and RAM-only drafts.
#include "api_client.h"
#include <QLocalServer>
#include <QLocalSocket>
#include <QtNetwork>
#include <QtWidgets>
#include <functional>
#include <signal.h>
#include <unistd.h>

static const QString Blue = "#0b5cad";
static QString str(const QJsonObject &object, const char *key) {
  return object.value(key).toString();
}
static QIcon appIcon() {
  const QString file = QCoreApplication::applicationDirPath() + "/spearhead.png";
  if (QFileInfo::exists(file)) {
    QIcon icon(file);
    if (!icon.isNull())
      return icon;
  }
  QPixmap p(64, 64);
  p.fill(Qt::transparent);
  QPainter q(&p);
  q.setRenderHint(QPainter::Antialiasing);
  q.setPen(Qt::NoPen);
  q.setBrush(QColor(Blue));
  q.drawRoundedRect(1, 1, 62, 62, 15, 15);
  QPainterPath path;
  path.moveTo(32, 10);
  path.lineTo(52, 40);
  path.lineTo(36, 34);
  path.lineTo(36, 54);
  path.lineTo(28, 54);
  path.lineTo(28, 34);
  path.lineTo(12, 40);
  path.closeSubpath();
  q.fillPath(path, QColor("#c0c0c0"));
  return QIcon(p);
}

// The native QML window runs in a disposable process; closing it releases
// its models and engine while the authenticated service and tray stay available.
class DesktopHost : public QObject {
public:
  Api api;
  QProcess renderer;
  QSystemTrayIcon tray;
  QMenu trayMenu;
  QLocalServer localServer;
  QTimer activityTimer;
  QJsonObject session;
  QString serverName;
  QByteArray output;
  qint64 rendererGroup = 0, lastActivity = -1;
  bool quitting = false, polling = false, ready = false, presentPending = false;
  static QString windowPath() {
    return QCoreApplication::applicationDirPath() + "/spearhead-window";
  }
  explicit DesktopHost(const QUrl &url, bool background, const QString &name)
      : api(url), tray(appIcon()), serverName(name) {
    QSettings settings;
    session = {{"theme", settings.value("ui/theme", "system").toString()},
               {"fontSize", settings.value("ui/fontSize", 100).toInt()},
               {"notifications", settings.value("ui/notifications", true).toBool()},
               {"closeToTray", settings.value("ui/closeToTray", true).toBool()},
               {"width", settings.value("ui/width", 1120).toInt()},
               {"height", settings.value("ui/height", 760).toInt()}};
    auto open = trayMenu.addAction("Open SpearHead");
    auto quit = trayMenu.addAction("Quit SpearHead");
    tray.setContextMenu(&trayMenu);
    tray.setToolTip("SpearHead — private LAN chat");
    tray.show();
    connect(open, &QAction::triggered, this, [this] { restore(); });
    connect(quit, &QAction::triggered, this, [this] { quitApp(); });
    connect(&tray, &QSystemTrayIcon::activated, this, [this](QSystemTrayIcon::ActivationReason r) {
      if (r == QSystemTrayIcon::Trigger || r == QSystemTrayIcon::DoubleClick)
        restore();
    });
    connect(&renderer, &QProcess::readyReadStandardOutput, this, [this] {
      output += renderer.readAllStandardOutput();
      if (output.size() > 8 * 1024 * 1024) {
        output.clear();
        return;
      }
      int end;
      while ((end = output.indexOf('\n')) >= 0) {
        auto event = QJsonDocument::fromJson(output.left(end)).object();
        output.remove(0, end + 1);
        QString kind = str(event, "event");
        if (kind == "closed")
          ready = false;
        if (kind == "ready") {
          ready = true;
          if (presentPending)
            restore();
        } else if (kind == "preferences") {
          QString theme = str(event, "theme");
          int size = event.value("fontSize").toInt();
          if (QStringList{"system", "light", "dark"}.contains(theme) && size >= 80 && size <= 160) {
            session["theme"] = theme;
            session["fontSize"] = size;
            QSettings preferences;
            preferences.setValue("ui/theme", theme);
            preferences.setValue("ui/fontSize", size);
            bool notifications = event.value("notifications").toBool(true);
            preferences.setValue("ui/notifications", notifications);
            session["notifications"] = notifications;
            bool closeToTray = event.value("closeToTray").toBool(true);
            preferences.setValue("ui/closeToTray", closeToTray);
            session["closeToTray"] = closeToTray;
          }
        } else if (kind == "closed") {
          session = event.value("session").toObject();
          QSettings preferences;
          if (QStringList{"system", "light", "dark"}.contains(str(session, "theme")))
            preferences.setValue("ui/theme", str(session, "theme"));
          for (const char *key : {"fontSize", "width", "height"})
            preferences.setValue(QString("ui/") + key, session.value(key).toInt());
          for (const char *key : {"notifications", "closeToTray"})
            preferences.setValue(QString("ui/") + key, session.value(key).toBool(true));
          if (!session.value("closeToTray").toBool(true))
            quitApp();
        } else if (kind == "notification" || kind == "error") {
          if (session.value("notifications").toBool(true) || kind == "error")
            tray.showMessage("SpearHead", str(event, "text"), QSystemTrayIcon::Information, 4000);
        }
      }
    });
    connect(&renderer, &QProcess::readyReadStandardError, this, [this] {
      // Renderer diagnostics never include the private input payload.
      auto diagnostic = renderer.readAllStandardError();
      fwrite(diagnostic.constData(), 1, size_t(diagnostic.size()), stderr);
    });
    connect(&renderer, QOverload<int, QProcess::ExitStatus>::of(&QProcess::finished), this,
            [this](int code, QProcess::ExitStatus status) {
              cleanupGroup(SIGTERM);
              ready = false;
              if (quitting)
                return;
              if (!session.value("closeToTray").toBool(true) ||
                  !QSystemTrayIcon::isSystemTrayAvailable()) {
                quitApp();
                return;
              }
              if (code != 0 || status != QProcess::NormalExit)
                tray.showMessage("SpearHead", "The window stopped. Reopen it from the tray.",
                                 QSystemTrayIcon::Warning, 4000);
              if (presentPending)
                QTimer::singleShot(0, this, [this] { restore(); });
            });
    QLocalServer::removeServer(serverName);
    localServer.listen(serverName);
    connect(&localServer, &QLocalServer::newConnection, this, [this] {
      auto socket = localServer.nextPendingConnection();
      connect(socket, &QLocalSocket::readyRead, socket, [this, socket] {
        socket->readAll();
        restore();
        socket->disconnectFromServer();
      });
      connect(socket, &QLocalSocket::disconnected, socket, &QObject::deleteLater);
    });
    connect(&activityTimer, &QTimer::timeout, this, [this] { poll(); });
    activityTimer.start(15000);
    poll();
    if (!background || !QSystemTrayIcon::isSystemTrayAvailable())
      restore();
  }
  ~DesktopHost() override {
    quitting = true;
    cleanupGroup(SIGTERM);
    if (!renderer.waitForFinished(2000) && renderer.state() != QProcess::NotRunning) {
      cleanupGroup(SIGKILL);
      renderer.kill();
      renderer.waitForFinished(1000);
    }
  }
  void cleanupGroup(int signal) {
    if (rendererGroup > 0) {
      ::kill(-pid_t(rendererGroup), signal);
      if (renderer.state() == QProcess::NotRunning)
        rendererGroup = 0;
    }
  }
  void restore() {
    if (quitting)
      return;
    if (renderer.state() != QProcess::NotRunning) {
      presentPending = !ready;
      if (ready)
        ::kill(pid_t(renderer.processId()), SIGUSR1);
      return;
    }
    presentPending = false;
    ready = false;
    output.clear();
    renderer.start(windowPath(), {"--window"});
    if (!renderer.waitForStarted(3000)) {
      tray.showMessage("SpearHead", "Could not start the native window. Reopen SpearHead.",
                       QSystemTrayIcon::Warning, 4000);
      return;
    }
    rendererGroup = renderer.processId();
    QUrl privateURL = api.base;
    QUrlQuery query;
    query.addQueryItem("t", api.token);
    privateURL.setQuery(query);
    QJsonObject payload = {
        {"url", privateURL.toString()},
        {"session", session},
        {"dark", QApplication::palette().color(QPalette::Window).lightness() < 128}};
    renderer.write(QJsonDocument(payload).toJson(QJsonDocument::Compact) + '\n');
    renderer.closeWriteChannel();
  }
  void poll() {
    if (polling || quitting)
      return;
    polling = true;
    api.get("/api/activity", [this](int code, QJsonObject data) {
      polling = false;
      if (code != 200)
        return;
      qint64 now = data.value("events").toVariant().toLongLong();
      if (renderer.state() == QProcess::NotRunning && lastActivity >= 0 && now > lastActivity &&
          session.value("notifications").toBool(true))
        tray.showMessage("SpearHead", "New messages or contact requests.",
                         QSystemTrayIcon::Information, 3000);
      lastActivity = now;
    });
  }
  void quitApp() {
    if (quitting)
      return;
    quitting = true;
    activityTimer.stop();
    cleanupGroup(SIGTERM);
    api.post("/api/desktop/quit", {}, [](int, QJsonObject) { QApplication::quit(); });
    QTimer::singleShot(1500, qApp, &QCoreApplication::quit);
  }
};

int main(int argc, char **argv) {
  QApplication app(argc, argv);
  QFont font("Noto Sans");
  font.setPixelSize(16);
  app.setFont(font);
  app.setApplicationName("SpearHead");
  app.setApplicationVersion(SPEARHEAD_VERSION);
  app.setOrganizationName("SpearHead");
  app.setDesktopFileName("spearhead");
  app.setQuitOnLastWindowClosed(false);
  app.setWindowIcon(appIcon());
  QCommandLineParser p;
  p.addHelpOption();
  p.addVersionOption();
  p.addOption({"runtime-check", "Verify the bundled native UI runtime"});
  p.addOption({"url", "Private local backend URL", "url"});
  p.addOption({"background", "Start in the system tray"});
  p.addOption({"url-stdin", "Read the private local URL from standard input"});
  p.process(app);
  if (p.isSet("runtime-check")) {
    QProcess probe;
    probe.start(DesktopHost::windowPath(), {"--runtime-check"});
    if (!probe.waitForFinished(15000))
      return 3;
    auto output = probe.readAllStandardOutput();
    fwrite(output.constData(), 1, size_t(output.size()), stdout);
    auto diagnostic = probe.readAllStandardError();
    fwrite(diagnostic.constData(), 1, size_t(diagnostic.size()), stderr);
    return probe.exitStatus() == QProcess::NormalExit ? probe.exitCode() : 3;
  }
  QString address = p.value("url");
  if (p.isSet("url-stdin")) {
    QFile input;
    input.open(stdin, QIODevice::ReadOnly);
    address = QString::fromUtf8(input.readLine(4096)).trimmed();
  }
  QUrl url(address);
  if (url.scheme() != "http" || (url.host() != "127.0.0.1" && url.host() != "::1") ||
      url.port() <= 0 || QUrlQuery(url).queryItemValue("t").isEmpty()) {
    qWarning("A private numeric loopback URL is required.");
    return 2;
  }
  QString name =
      "spearhead-ui-" + QString(QCryptographicHash::hash(
                                    url.adjusted(QUrl::RemoveQuery | QUrl::RemovePath).toEncoded(),
                                    QCryptographicHash::Sha256)
                                    .toHex());
  // Reopen hosts from older installations which do not yet own a lock file.
  {
    QLocalSocket socket;
    socket.connectToServer(name);
    if (socket.waitForConnected(200)) {
      socket.write("show");
      if (socket.waitForBytesWritten(1000))
        return 0;
    }
  }
  // Reserve the host before the native window and its runtime initialize.
  QLockFile instanceLock(QDir::tempPath() + "/" + name + ".lock");
  instanceLock.setStaleLockTime(0);
  bool acquired = instanceLock.tryLock(0);
  if (!acquired) {
    QElapsedTimer deadline;
    deadline.start();
    while (deadline.elapsed() < 10000) {
      QLocalSocket socket;
      socket.connectToServer(name);
      if (socket.waitForConnected(200)) {
        socket.write("show");
        if (socket.waitForBytesWritten(1000))
          return 0;
      }
      acquired = instanceLock.tryLock(0);
      if (acquired)
        break;
      QThread::msleep(100);
    }
    if (!acquired) {
      qWarning("The existing SpearHead desktop did not respond.");
      return 4;
    }
  }
  DesktopHost host(url, p.isSet("background"), name);
  return app.exec();
}
