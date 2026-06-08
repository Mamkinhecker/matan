# Matan Trainer

Простой Go-сайт для подготовки студентов к письменным вариантам и устному тесту по матану.

## Запуск

Нужен PostgreSQL. По умолчанию приложение ждёт:

```text
postgres://postgres:postgres@localhost:5432/matan_trainer?sslmode=disable
```

Можно переопределить:

```sh
export DATABASE_URL='postgres://user:password@localhost:5432/matan_trainer?sslmode=disable'
go run ./cmd/server
```

При старте приложение само создаёт таблицы и засевает демо-данные, если база пустая.

Страницы:

- `/` — обзор
- `/topics` — темы
- `/cards` — карточки формулировок
- `/formulations` — тренажер письменного воспроизведения
- `/proof-game` — игра “выбери доказательство”
- `/oral` — устный тест
- `/variant` — письменный вариант
- `/progress` — прогресс
