#include "nativeicon.h"
#include <QPainter>
#include <QPainterPath>
#include <QtCore>
void NativeIcon::reload() {
  static QJsonObject icons = [] {
    QFile file(":/ui/assets/icons.json");
    file.open(QIODevice::ReadOnly);
    return QJsonDocument::fromJson(file.readAll()).object();
  }();
  QByteArray svg = icons.value(m_name).toString().replace("currentColor", m_color.name()).toUtf8();
  if (m_name != "logo" && !svg.isEmpty())
    m_svg.load(svg);
  update();
  emit changed();
}
void NativeIcon::paint(QPainter *p) {
  p->setRenderHint(QPainter::Antialiasing);
  if (m_name != "logo") {
    m_svg.render(p, QRectF(0, 0, width(), height()));
    return;
  }
  p->scale(width() / 32, height() / 32);
  p->setPen(Qt::NoPen);
  p->setBrush(QColor("#0b5cad"));
  p->drawRoundedRect(QRectF(0, 0, 32, 32), 8, 8);
  QPainterPath shape;
  shape.moveTo(16, 5);
  shape.lineTo(26, 20);
  shape.lineTo(18, 17);
  shape.lineTo(18, 27);
  shape.lineTo(14, 27);
  shape.lineTo(14, 17);
  shape.lineTo(6, 20);
  shape.closeSubpath();
  p->fillPath(shape, QColor("#c0c0c0"));
}
void NativeBubble::paint(QPainter *p) {
  p->setRenderHint(QPainter::Antialiasing);
  qreal w = width() - 1, h = height() - 1, r = qMin(m_rounding, qMin(w, h) / 2),
        bl = m_outgoing ? r : qMin<qreal>(4, r), br = m_outgoing ? qMin<qreal>(4, r) : r;
  p->translate(.5, .5);
  QPainterPath shape;
  shape.moveTo(r, 0);
  shape.lineTo(w - r, 0);
  shape.quadTo(w, 0, w, r);
  shape.lineTo(w, h - br);
  shape.quadTo(w, h, w - br, h);
  shape.lineTo(bl, h);
  shape.quadTo(0, h, 0, h - bl);
  shape.lineTo(0, r);
  shape.quadTo(0, 0, r, 0);
  shape.closeSubpath();
  p->setPen(QPen(m_stroke, 1));
  if (m_outgoing) {
    QLinearGradient gradient(0, 0, w, h);
    gradient.setColorAt(0, QColor("#0b5cad"));
    gradient.setColorAt(1, QColor("#094e96"));
    p->setBrush(gradient);
  } else
    p->setBrush(m_color);
  p->drawPath(shape);
}
