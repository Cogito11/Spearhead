#pragma once
#include <QApplication>
#include <QtGui>
inline void initialiseFonts() {
  static bool ready = false;
  if (ready)
    return;
  ready = true;
  QString root = qEnvironmentVariable("SPEARHEAD_APPDIR");
  QString folder = root.isEmpty() ? QString("/usr/share/fonts/truetype/noto")
                                  : root + "/usr/share/fonts/truetype/noto";
  for (const QString &style :
       {QString("Regular"), QString("Bold"), QString("Italic"), QString("BoldItalic")})
    QFontDatabase::addApplicationFont(folder + "/NotoSans-" + style + ".ttf");
}
inline void applyNativePalette(bool dark) {
  QPalette palette;
  auto bg = QColor(dark ? "#0c1423" : "#eef2f8"), surface = QColor(dark ? "#16243a" : "#ffffff"),
       fg = QColor(dark ? "#edf3fc" : "#17213a"), muted = QColor(dark ? "#abbcd2" : "#576980"),
       primary = QColor(dark ? "#94c7f5" : "#0b5cad"), on = QColor(dark ? "#0d1b2f" : "#ffffff");
  palette.setColor(QPalette::Window, bg);
  palette.setColor(QPalette::Base, surface);
  palette.setColor(QPalette::AlternateBase, bg);
  palette.setColor(QPalette::Button, surface);
  palette.setColor(QPalette::WindowText, fg);
  palette.setColor(QPalette::Text, fg);
  palette.setColor(QPalette::ButtonText, fg);
  palette.setColor(QPalette::PlaceholderText, muted);
  palette.setColor(QPalette::Highlight, primary);
  palette.setColor(QPalette::HighlightedText, on);
  palette.setColor(QPalette::ToolTipBase, surface);
  palette.setColor(QPalette::ToolTipText, fg);
  QApplication::setPalette(palette);
}
