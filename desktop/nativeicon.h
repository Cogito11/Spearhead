#pragma once
#include <QQuickPaintedItem>
#include <QSvgRenderer>
class NativeIcon : public QQuickPaintedItem {
  Q_OBJECT
  Q_PROPERTY(QString name READ name WRITE setName NOTIFY changed)
  Q_PROPERTY(QColor color READ color WRITE setColor NOTIFY changed)
public:
  NativeIcon(QQuickItem *parent = nullptr) : QQuickPaintedItem(parent) {
    setAntialiasing(true);
  }
  QString name() const {
    return m_name;
  }
  QColor color() const {
    return m_color;
  }
  void setName(const QString &value) {
    if (m_name != value) {
      m_name = value;
      reload();
    }
  }
  void setColor(const QColor &value) {
    if (m_color != value) {
      m_color = value;
      reload();
    }
  }
  void paint(QPainter *painter) override;
signals:
  void changed();

private:
  QString m_name;
  QColor m_color = "#576980";
  QSvgRenderer m_svg;
  void reload();
};
class NativeBubble : public QQuickPaintedItem {
  Q_OBJECT
  Q_PROPERTY(QColor color READ color WRITE setColor NOTIFY changed)
  Q_PROPERTY(QColor stroke READ stroke WRITE setStroke NOTIFY changed)
  Q_PROPERTY(bool outgoing READ outgoing WRITE setOutgoing NOTIFY changed)
  Q_PROPERTY(qreal rounding READ rounding WRITE setRounding NOTIFY changed)
public:
  NativeBubble(QQuickItem *parent = nullptr) : QQuickPaintedItem(parent) {
    setAntialiasing(true);
  }
  QColor color() const {
    return m_color;
  }
  QColor stroke() const {
    return m_stroke;
  }
  bool outgoing() const {
    return m_outgoing;
  }
  qreal rounding() const {
    return m_rounding;
  }
  void setColor(QColor v) {
    m_color = v;
    update();
    emit changed();
  }
  void setStroke(QColor v) {
    m_stroke = v;
    update();
    emit changed();
  }
  void setOutgoing(bool v) {
    m_outgoing = v;
    update();
    emit changed();
  }
  void setRounding(qreal v) {
    m_rounding = v;
    update();
    emit changed();
  }
  void paint(QPainter *p) override;
signals:
  void changed();

private:
  QColor m_color, m_stroke;
  bool m_outgoing = false;
  qreal m_rounding = 18;
};
