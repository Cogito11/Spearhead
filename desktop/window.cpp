#include "bridge.h"
#include "nativeicon.h"
#include <QApplication>
#include <QLocalSocket>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickStyle>
#include <QQuickWindow>
#include <fcntl.h>
#include <signal.h>
#include <unistd.h>
static int wakePipe[2] = {-1, -1};
static void wakeSignal(int) {
  char value = 1;
  if (wakePipe[1] >= 0) {
    auto ignored = ::write(wakePipe[1], &value, 1);
    (void)ignored;
  }
}
static int runWindow(bool check) {
  QJsonObject payload;
  if (!check) {
    ::setsid();
    QFile input;
    input.open(stdin, QIODevice::ReadOnly);
    auto document = QJsonDocument::fromJson(input.readLine(8 * 1024 * 1024));
    payload = document.object();
    QUrl url(payload.value("url").toString());
    if (url.scheme() != "http" || (url.host() != "127.0.0.1" && url.host() != "::1") ||
        url.port() <= 0 || QUrlQuery(url).queryItemValue("t").isEmpty())
      return 2;
  }
  Bridge bridge(payload);
  qmlRegisterType<NativeIcon>("SpearHead", 1, 0, "NativeIcon");
  qmlRegisterType<NativeBubble>("SpearHead", 1, 0, "NativeBubble");
  QQmlApplicationEngine engine;
  engine.rootContext()->setContextProperty("backend", &bridge);
  engine.rootContext()->setContextProperty(
      "initialWidth", qMax(400, payload.value("session").toObject().value("width").toInt(1120)));
  engine.rootContext()->setContextProperty(
      "initialHeight", qMax(400, payload.value("session").toObject().value("height").toInt(760)));
  engine.rootContext()->setContextProperty("runtimeCheck", check);
  engine.load(QUrl("qrc:/ui/qml/Main.qml"));
  if (engine.rootObjects().isEmpty())
    return 3;
  if (check) {
    fprintf(stdout, "Qt %s / native Qt Quick QML UI\n", qVersion());
    return 0;
  }
  auto window = qobject_cast<QQuickWindow *>(engine.rootObjects().first());
  if (!window)
    return 3;
  if (::pipe2(wakePipe, O_NONBLOCK | O_CLOEXEC) == 0) {
    ::signal(SIGUSR1, wakeSignal);
    auto notifier = new QSocketNotifier(wakePipe[0], QSocketNotifier::Read, &bridge);
    QObject::connect(notifier, &QSocketNotifier::activated, &bridge, [window] {
      char bytes[32];
      while (::read(wakePipe[0], bytes, sizeof(bytes)) > 0) {
      }
      window->showNormal();
      window->raise();
      window->requestActivate();
    });
  }
  bridge.start();
  Bridge::event({{"event", "ready"}});
  int result = QCoreApplication::exec();
  if (wakePipe[0] >= 0)
    ::close(wakePipe[0]);
  if (wakePipe[1] >= 0)
    ::close(wakePipe[1]);
  wakePipe[0] = wakePipe[1] = -1;
  return result;
}

int main(int argc, char **argv) {
  QQuickWindow::setSceneGraphBackend("software");
  QQuickStyle::setStyle("Basic");
  QApplication app(argc, argv);
  app.setApplicationName("SpearHead");
  app.setApplicationVersion(SPEARHEAD_VERSION);
  app.setOrganizationName("SpearHead");
  app.setDesktopFileName("spearhead");
  app.setWindowIcon(QIcon(":/ui/assets/brand.png"));
  QFont font("Noto Sans");
  font.setPixelSize(16);
  app.setFont(font);
  QCommandLineParser parser;
  parser.addHelpOption();
  parser.addVersionOption();
  parser.addOption({"window", "Run the native window"});
  parser.addOption({"runtime-check", "Verify the native runtime"});
  parser.process(app);
  return runWindow(parser.isSet("runtime-check"));
}
