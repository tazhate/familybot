# FamilyBot — Семейный планировщик

Telegram-бот для управления задачами, напоминаниями и расписанием семьи.

## Анализ требований (на основе life-plan)

### Типы задач
1. **Одноразовые** — с приоритетом (срочно / на неделе / когда-нибудь)
2. **Регулярные** — повторяющиеся по расписанию
3. **Привязанные к дате** — дни рождения, события

### Типы напоминаний
| Тип | Пример | Логика |
|-----|--------|--------|
| Ежедневные | Статус Allnodes утром/вечером | Каждый день в X:XX |
| Еженедельные | Дежурство вторник, психолог Пн 16:00 | День недели + время |
| Ежемесячные | Отчёт Apostol 4-е число | День месяца |
| По неделе месяца | Дежурство Пт 2-й недели | Неделя + день |
| Плавающие | Лука Сб или Вс | Требует подтверждения |
| Годовые | Дни рождения | Дата в году |

### Пользователи и доступ
- **Денис** — основной пользователь
- **Ира** — партнёр, свои задачи + общие семейные
- Задачи: личные / общие / назначенные другому

### Сущности из life-plan
- Люди (дети, родственники, контакты)
- Авто (ТО, страховки)
- Проекты
- Здоровье (врачи, лекарства)
- Финансы
- Юридические дела

---

## Архитектура

### Стек
- **Go 1.25+** (текущая стабильная 1.25.6, скоро выйдет 1.26)
- **SQLite** (mattn/go-sqlite3 v1.14+) — для начала, потом можно PostgreSQL
- **telegram-bot-api v5** (github.com/go-telegram-bot-api/telegram-bot-api/v5)
- **cron v3** (github.com/robfig/cron/v3) — для scheduler
- **Docker** + **Kubernetes** — деплой

### Структура проекта
```
familybot/
├── cmd/
│   └── bot/
│       └── main.go
├── internal/
│   ├── bot/
│   │   ├── bot.go
│   │   ├── handlers.go
│   │   └── commands.go
│   ├── domain/
│   │   ├── user.go
│   │   ├── task.go
│   │   ├── reminder.go
│   │   ├── schedule.go
│   │   └── person.go
│   ├── storage/
│   │   ├── sqlite.go
│   │   └── migrations/
│   ├── scheduler/
│   │   └── scheduler.go
│   └── service/
│       ├── task_service.go
│       └── reminder_service.go
├── config/
│   └── config.go
├── k8s/
│   ├── namespace.yaml
│   ├── secret.yaml
│   ├── configmap.yaml
│   ├── pvc.yaml
│   ├── deployment.yaml
│   ├── service.yaml
│   └── ingress.yaml
├── .github/
│   └── workflows/
│       └── deploy.yaml
├── migrations/
├── Dockerfile
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### Модели данных

```go
// User — пользователь бота
type User struct {
    ID         int64
    TelegramID int64
    Name       string
    Role       string // "owner", "partner"
    CreatedAt  time.Time
}

// Task — задача
type Task struct {
    ID          int64
    UserID      int64      // создатель
    AssignedTo  *int64     // кому назначена (nil = себе)
    Title       string
    Description string
    Priority    string     // "urgent", "week", "someday"
    IsShared    bool       // общая семейная
    DueDate     *time.Time
    DoneAt      *time.Time
    CreatedAt   time.Time
}

// Reminder — напоминание
type Reminder struct {
    ID        int64
    UserID    int64
    Title     string
    Schedule  string    // cron expression или custom
    Type      string    // "daily", "weekly", "monthly", "yearly", "custom"
    Params    string    // JSON с параметрами (день недели, неделя месяца, etc)
    IsActive  bool
    LastSent  *time.Time
    CreatedAt time.Time
}

// Person — человек (ребёнок, родственник, контакт)
type Person struct {
    ID        int64
    UserID    int64
    Name      string
    Role      string     // "child", "family", "contact"
    Birthday  *time.Time
    Notes     string
    CreatedAt time.Time
}
```

### Команды бота

```
/start              — регистрация
/add <текст>        — добавить задачу
/list               — список задач
/done <id>          — выполнить задачу
/remind <текст>     — добавить напоминание
/reminders          — список напоминаний
/today              — что на сегодня
/week               — расписание недели
/birthdays          — ближайшие дни рождения
/shared             — общие семейные задачи
/assign <id> <user> — назначить задачу
```

### Inline-режим и кнопки
- Быстрое добавление задач
- Выбор приоритета кнопками
- Отметка выполнения одной кнопкой
- Подтверждение плавающих событий (Лука Сб/Вс?)

---

## План реализации

### Фаза 1: MVP (базовый функционал)
- [ ] Инициализация проекта (go mod, структура)
- [ ] Подключение к Telegram Bot API
- [ ] SQLite + базовые миграции
- [ ] Регистрация пользователя (/start)
- [ ] CRUD задач (/add, /list, /done)
- [ ] Базовый scheduler для напоминаний

### Фаза 2: Расписание
- [ ] Еженедельное расписание (/week)
- [ ] Ежедневные напоминания (утро/вечер)
- [ ] Еженедельные напоминания (день недели)
- [ ] Ежемесячные (4-е число)
- [ ] По неделе месяца (2-я пятница)

### Фаза 3: Многопользовательность
- [ ] Добавление партнёра (Ира)
- [ ] Общие семейные задачи
- [ ] Назначение задач друг другу
- [ ] Раздельные/общие напоминания

### Фаза 4: Расширенные функции
- [ ] Люди (дети, родственники) с ДР
- [ ] Напоминания о днях рождения
- [ ] Авто (ТО, страховки)
- [ ] Интеграция с Google Calendar (опционально)

### Фаза 5: UX улучшения
- [ ] Inline-кнопки везде
- [ ] Быстрый ввод голосом (speech-to-text)
- [ ] Группировка по контексту
- [ ] Статистика выполнения

---

## Особенности для СДВГ

1. **Минимум шагов** — одно сообщение = одна задача
2. **Утренний брифинг** — автоматом в 9:00 что на сегодня
3. **Вечерний чекин** — что сделал, что перенести
4. **Повторные напоминания** — если не отметил, напомнить снова
5. **Визуальные маркеры** — эмодзи для приоритетов
6. **Контекстные подсказки** — "Через 30 мин созвон Allnodes"

---

## Конфигурация

Через переменные окружения (удобно для K8s):

```bash
# Telegram
TELEGRAM_BOT_TOKEN=xxx
OWNER_TELEGRAM_ID=123456789      # Денис
PARTNER_TELEGRAM_ID=987654321    # Ира

# Database
DATABASE_PATH=/data/familybot.db

# Reminders
MORNING_TIME=09:00
EVENING_TIME=21:00
TIMEZONE=Europe/Moscow

# Server (для webhook-режима)
WEBHOOK_URL=https://family.tazhate.com
SERVER_PORT=8080
```

---

## Dockerfile

```dockerfile
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /familybot ./cmd/bot

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata sqlite

COPY --from=builder /familybot /familybot

EXPOSE 8080
VOLUME ["/data"]

CMD ["/familybot"]
```

---

## Запуск

```bash
# Разработка
make run

# Сборка
make build

# Docker образ
make docker-build
make docker-push
```

---

## Деплой в Kubernetes

### Домен
- **Production:** `family.tazhate.com`

### Манифесты

```
k8s/
├── namespace.yaml
├── secret.yaml           # BOT_TOKEN и др.
├── configmap.yaml        # конфиг приложения
├── pvc.yaml              # PersistentVolumeClaim для SQLite
├── deployment.yaml
├── service.yaml
└── ingress.yaml          # family.tazhate.com
```

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: familybot
  namespace: familybot
spec:
  replicas: 1              # Telegram long-polling = 1 реплика
  selector:
    matchLabels:
      app: familybot
  template:
    metadata:
      labels:
        app: familybot
    spec:
      containers:
      - name: familybot
        image: registry.tazhate.com/familybot:latest
        envFrom:
        - secretRef:
            name: familybot-secrets
        - configMapRef:
            name: familybot-config
        volumeMounts:
        - name: data
          mountPath: /data
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "256Mi"
            cpu: "200m"
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: familybot-data
```

### Ingress (для webhook-режима)

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: familybot
  namespace: familybot
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - family.tazhate.com
    secretName: familybot-tls
  rules:
  - host: family.tazhate.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: familybot
            port:
              number: 8080
```

### Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: familybot-secrets
  namespace: familybot
type: Opaque
stringData:
  TELEGRAM_BOT_TOKEN: "xxx"
  OWNER_TELEGRAM_ID: "123456789"
  PARTNER_TELEGRAM_ID: "987654321"
```

### Деплой команды

```bash
# Применить манифесты
kubectl apply -f k8s/

# Проверить статус
kubectl -n familybot get pods

# Логи
kubectl -n familybot logs -f deploy/familybot

# Обновить образ
kubectl -n familybot rollout restart deploy/familybot
```

### CI/CD (GitHub Actions)

```yaml
# .github/workflows/deploy.yaml
name: Deploy
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4

    - uses: actions/setup-go@v5
      with:
        go-version: '1.25'

    - name: Build & Push
      run: |
        docker build -t registry.tazhate.com/familybot:${{ github.sha }} .
        docker push registry.tazhate.com/familybot:${{ github.sha }}

    - name: Deploy to K8s
      run: |
        kubectl set image deploy/familybot \
          familybot=registry.tazhate.com/familybot:${{ github.sha }} \
          -n familybot
```
