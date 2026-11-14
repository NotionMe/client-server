# Практична робота № 5

## Тема: Розробка REST сервісу на базі фреймворку Gin та СУБД PostgreSQL

**Дисципліна:** Розробка застосувань клієнт-серверної архітектури.  
**Варіант:** не передбачено.

---

## Мета роботи

- Реалізувати REST API для обліку інцидентів засобами мови Go та фреймворку Gin.
- Опанувати десеріалізацію JSON і перевірку вхідних даних за допомогою DTO та тегів `binding`.
- Організувати збереження користувачів та інцидентів у PostgreSQL через ORM GORM.
- Реалізувати реєстрацію, перевірку паролів bcrypt, видачу JWT та захист маршрутів middleware.
- Сформувати документацію API за допомогою Swag і підключити Swagger UI.

## Технічне забезпечення

| Компонент | Версія / опис |
| --- | --- |
| ОС | Linux; робоче середовище Ryoku на базі Arch Linux |
| Мова та інструментарій | Go `1.27.3`; версію перевірено командою `go version` |
| HTTP-фреймворк | `github.com/gin-gonic/gin v1.12.0` |
| ORM | `gorm.io/gorm v1.31.2` |
| Драйвер PostgreSQL | `gorm.io/driver/postgres v1.6.3` |
| СУБД у конфігурації контейнера | Образ `postgres:17-alpine` з `docker-compose.yml`; це не підтвердження версії запущеної БД |
| Гешування паролів | bcrypt із `golang.org/x/crypto v0.59.0` |
| JWT | `github.com/golang-jwt/jwt/v5 v5.3.1` |
| Swagger | `swag v1.16.6` як Go-залежність, `gin-swagger v1.6.1`, `files v1.0.1` |
| Генератор документації | Установлений CLI `swag` повідомляє версію `v1.16.4` |
| Робота з HTTP | `curl`; для обробки JSON у прикладах — `jq` |
| Компіляція та статична перевірка | `go build`, `go test`, `go vet` |
| Система контролю версій | Git; каталог проєкту є робочим деревом репозиторію |

Версії бібліотек наведено за актуальним `go.mod`. Docker Compose використовується як опис способу запуску PostgreSQL; під час підготовки цього звіту контейнери не запускалися.

---

## Хід виконання роботи

### 1. Структура та потік обробки запитів

Проєкт реалізує сервер керування інцидентами. HTTP-запити приймає Gin, прикладні обробники виконують перевірку даних і звертаються до PostgreSQL через GORM. Окремих шарів сервісів і репозиторіїв у поточній реалізації немає: обробники отримують `*gorm.DB` безпосередньо.

```text
pr-5/
├── main.go
├── go.mod
├── go.sum
├── docker-compose.yml
├── auth/
│   └── claims.go
├── database/
│   └── config.go
├── entity/
│   ├── incident.go
│   ├── user.go
│   └── dto/
│       ├── incident_dto.go
│       └── user_dto.go
├── handlers/
│   ├── incedent_handler.go
│   └── user_handler.go
├── middleware/
│   └── auth.go
└── docs/
    ├── docs.go
    ├── swagger.json
    └── swagger.yaml
```

`main.go` налаштовує маршрути й запуск сервера, `entity` містить моделі БД, а `dto` — структури запитів. У `auth` визначено спільний формат JWT, у `middleware` — перевірку доступу. Назву файла `incedent_handler.go` наведено так, як вона записана у проєкті. Каталог `docs` створюється генератором Swag.

Для захищених операцій застосовано такий потік:

```text
HTTP-запит
    → AuthMiddleware: перевірка Bearer JWT
    → userID у контексті Gin
    → обробник: ShouldBindJSON та робота з GORM
    → PostgreSQL
    → HTTP-відповідь у JSON
```

Middleware виконується до обробника маршруту. Якщо авторизація не пройшла, ланцюжок переривається і операція з інцидентом не виконується.

### 2. Підключення PostgreSQL та моделі даних

Функція `database.StartDB` відкриває підключення через PostgreSQL-драйвер GORM, після чого викликає `AutoMigrate` для `entity.Incident` і `entity.User`. Помилка підключення або міграції передається в `main`, де запуск завершується через `log.Fatal`.

У поточному коді DSN задано безпосередньо в `main.go`: використовується локальна адреса `127.0.0.1`, порт `5432`, база та користувач `pr5`. Секретне значення пароля в цьому звіті не публікується. Змінні `POSTGRES_*` налаштовують контейнер, але застосунок **не читає їх автоматично**: після зміни параметрів контейнера необхідно узгодити DSN у програмі.

**Модель інциденту:**

| Поле JSON | Тип Go | Призначення |
| --- | --- | --- |
| `id` | `uint` | Первинний ключ |
| `user_id` | `uint` | ID користувача, від імені якого створено інцидент; поле індексується |
| `title` | `string` | Заголовок; у моделі задано розмір 100 і `not null` |
| `description` | `string` | Опис; колонка типу `text` |
| `priority` | `string` | Пріоритет інциденту |
| `status` | `string` | Статус, початкове значення `open` |
| `created_at` | `time.Time` | Час створення |

**Модель користувача:**

| Поле Go | Серіалізація JSON | Призначення |
| --- | --- | --- |
| `ID` | `id` | Первинний ключ |
| `Email` | `Email` | Email з унікальним індексом; явний JSON-тег не задано |
| `PasswordHash` | Не серіалізується (`json:"-"`) | Збережений bcrypt-геш |
| `CreatedAt` | `CreatedAt` | Час створення користувача |

Поле `Incident.UserID` зберігає автора, але в моделі не оголошено асоціацію з `User` або зовнішній ключ. Також `user_id` не має обмеження `not null`. Старим інцидентам автор автоматично не призначається.

### 3. DTO та валідація JSON

Моделі БД відокремлено від DTO. Це дозволяє приймати тільки передбачені поля запиту та не покладатися на переданий клієнтом ID автора.

| DTO | Поля та правила |
| --- | --- |
| `CreateIncidentDTO` | `title`: обов'язкове, 5–100 символів; `description`: обов'язкове; `priority`: `low`, `medium`, `high` або `critical` |
| `UpdateIncidentDTO` | `status`: обов'язкове, одне з `open`, `in_progress`, `resolved`, `closed` |
| `RegisterDTO` | `email`: обов'язкове, формат email; `password`: обов'язкове, мінімум 6 символів |
| `LoginDTO` | Аналогічна перевірка формату email та мінімальної довжини пароля |

Обробники викликають `ShouldBindJSON`. У разі помилки формують відповідь `400` і завершують функцію через `return`. У поточній версії помилки валідації повертаються як рядок `err.Error()`, а не як окремий масив полів. У відповідях обробників реєстрації та входу на цьому шляху ключ має фактичну назву `error:` із двокрапкою; в інших місцях використовується `error`.

### 4. Маршрути API та операції з інцидентами

Базовий префікс API — `/api/v1`.

| Метод | Маршрут | Доступ | Успішний результат |
| --- | --- | --- | --- |
| GET | `/api/v1/ping` | Публічний | `200`, повідомлення `pong` |
| POST | `/api/v1/auth/register` | Публічний | `201`, дані користувача без гешу |
| POST | `/api/v1/auth/login` | Публічний | `200`, JWT у полі `token` |
| GET | `/api/v1/incidents` | Публічний | `200`, список інцидентів |
| GET | `/api/v1/incidents/:id` | Публічний | `200`, один інцидент |
| POST | `/api/v1/incidents` | Bearer JWT | `201`, створений інцидент |
| PATCH | `/api/v1/incidents/:id/status` | Bearer JWT | `200`, дані інциденту після оновлення |
| DELETE | `/api/v1/incidents/:id` | Bearer JWT | `204`, без тіла відповіді |
| GET | `/swagger/index.html` | Публічний | Swagger UI |
| GET | `/swagger/doc.json` | Публічний | Згенерований опис API |

При створенні інциденту обробник отримує `userID` з контексту, перевіряє його тип і ненульове значення, читає DTO та заповнює `entity.Incident`. Помилка `db.Create` повертає `500`; успішне збереження завершується відповіддю `201`.

Оновлення статусу спочатку перевіряє DTO, знаходить інцидент і виконує `db.Model(&incident).Update(...)`. Видалення також спочатку шукає запис за ID. Поточні особливості обробки помилок цих операцій наведено в підрозділі 11 — їх не слід вважати повністю уніфікованими.

### 5. Реєстрація та перевірка облікових даних

Реєстрація складається з читання `RegisterDTO`, створення bcrypt-гешу через `GenerateFromPassword` з `DefaultCost`, заповнення `entity.User` і виклику GORM `Create`. Пароль відкритим текстом у моделі користувача не зберігається, а поле гешу приховано від JSON.

Вхід виконує параметризований пошук за email через `Where("email = ?", loginUser.Email).First(&user)`. Відсутній користувач отримує `401`; інша помилка читання БД — `500`. Потім `CompareHashAndPassword` порівнює збережений геш із паролем із запиту. Для відсутнього користувача та неправильного пароля повертається однакове повідомлення «Неправильний email або пароль».

### 6. Формування JWT

Спільна структура `auth.Claims` містить `UserID` і вбудовані `jwt.RegisteredClaims`. Функція `createAccessToken` формує такі claims:

| Claim | Значення |
| --- | --- |
| `user_id` | ID користувача як ціле число |
| `sub` | Той самий ID у вигляді рядка |
| `iat` | Час створення токена |
| `exp` | Час створення плюс 24 години |

Для підпису використовується HS256. Секрет зчитується зі змінної середовища `JWT_SECRET`; застосунок перевіряє мінімальну довжину 32 байти. Відсутній або короткий секрет перериває запуск сервера. Після успішного підписання login повертає `200` з об'єктом, що містить `token`. Пароль і його геш у claims не включаються.

### 7. JWT Auth Middleware і захищена група

`AuthMiddleware(secretKey string)` повертає `gin.HandlerFunc` і виконує такі дії:

1. Читає заголовок `Authorization` та перевіряє точний префікс `Bearer `.
2. Вилучає рядок токена та відхиляє порожнє значення.
3. Викликає `jwt.ParseWithClaims` зі спільною структурою claims і секретним ключем.
4. Обмежує допустимий алгоритм до HS256 та вимагає наявності `exp`.
5. Відхиляє помилку парсингу, невалідний токен або нульовий `UserID`.
6. Записує `claims.UserID` у контекст через `c.Set("userID", ...)` та викликає `c.Next()`.

Для помилки авторизації використовується `AbortWithStatusJSON(401, ...)` з відповіддю:

```json
{"error":"Unauthorized access"}
```

Після формування такої відповіді middleware виконує `return`. У `main.go` окрема група `private` застосовує цей middleware до POST, PATCH і DELETE для інцидентів. Публічні GET та маршрути auth реєструються без нього.

Автор нового інциденту визначається перевіреним токеном. Водночас зміна статусу та видалення наразі доступні будь-якому авторизованому користувачеві: перевірка належності інциденту конкретному автору не реалізована.

### 8. Swagger UI та генерація документації

Над `main()` наведено назву API, версію, базовий шлях і схему `BearerAuth` типу `apiKey` у заголовку `Authorization`. Обробники мають анотації `@Summary`, `@Tags`, `@Param`, `@Success`, `@Failure`, `@Router`, а захищені операції — `@Security BearerAuth`.

```bash
go install github.com/swaggo/swag/cmd/swag@latest
"$(go env GOPATH)/bin/swag" init
```

Перша команда встановлює CLI, друга запускає його з кореня проєкту та створює `docs/docs.go`, `docs/swagger.json` і `docs/swagger.yaml`. Абсолютний шлях через `GOPATH` дозволяє запускати генератор, навіть якщо каталог його бінарних файлів не додано до `PATH`. Для відтворення встановлення залежності сервера вже зафіксовано в `go.mod`; `@latest` стосується окремої CLI-утиліти.

У `main.go` пакет `notion_pr5/docs` імпортується для реєстрації опису API. Маршрут `/swagger/*any` підключає `ginSwagger.WrapHandler(swaggerFiles.Handler)`. Після запуску UI доступний за адресою `http://localhost:8080/swagger/index.html`. У вікні **Authorize** необхідно ввести повний рядок `Bearer <отриманий JWT>`.

Позначка `@Security` документує захист, але не замінює middleware. Після зміни анотацій документацію потрібно згенерувати повторно.

### 9. Порядок локального запуску

Команди нижче виконуються з кореня `pr-5` у bash-сумісній оболонці.

```bash
go mod download
docker compose up -d postgres
docker compose ps
```

Завантажуються Go-залежності та запускається сервіс PostgreSQL з Compose. Конфігурація використовує локальний порт, іменований том `postgres_data` і перевірку готовності через `pg_isready`. Перед запуском API необхідно дочекатися готовності БД та переконатися, що її параметри відповідають DSN у `main.go`. Наявність і працездатність Docker у середовищі читача цими командами не припускаються перевіреними.

```bash
export JWT_SECRET="$(openssl rand -hex 32)"
go run .
```

Секрет установлюється в середовище поточної оболонки, після чого запускається сервер. Команду генерації секрету слід виконати один раз для локального сеансу; для збереження дійсності токенів між перезапусками потрібно використовувати той самий секрет. Новий ключ робить попередні підписи недійсними. За відсутності налаштування `PORT` Gin використовує порт `8080`; приклади нижче розраховані на нього.

Під час старту GORM виконує міграцію моделей. Зокрема, оновлення моделі інциденту додає поле `user_id` до схеми, але не встановлює автора старим записам.

### 10. Приклади перевірки API

Наведені команди є прикладами для ручної перевірки, а не протоколом виконаних під час оформлення звіту HTTP-запитів. Успішні статуси в таблиці маршрутів описують призначення обробників, а не доводять успіх конкретної операції з БД.

#### 10.1. Реєстрація

```bash
curl -i -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"student@example.com","password":"testpass123"}'
```

Запит передає дані `RegisterDTO`. Він створює користувача за умови успішного гешування та запису. Для повторної перевірки слід враховувати унікальність email; поточний обробник не перевіряє помилку `Create`, тому сам статус `201` не є достатнім доказом збереження.

#### 10.2. Вхід і отримання токена

```bash
TOKEN="$(curl -fsS -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"student@example.com","password":"testpass123"}' \
  | jq -er '.token')"
```

Для наявного користувача з правильним паролем з відповіді вилучається поле `token`. Для команди потрібен `jq`; змінна `TOKEN` використовується в наступних прикладах. Якщо login неуспішний, спочатку потрібно перевірити його відповідь, а не продовжувати запити з порожнім токеном.

#### 10.3. Створення інциденту

```bash
curl -i -X POST http://localhost:8080/api/v1/incidents \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"title":"Database unavailable","description":"Application cannot connect to PostgreSQL","priority":"high"}'
```

Успішний запит повертає `201` та модель інциденту з `id`, `user_id`, статусом і часом створення. Значення `user_id` береться з токена, тому клієнт не задає автора в JSON. **Потрібно зберегти отриманий ID** для оновлення й видалення саме цього запису.

#### 10.4. Публічний перегляд

```bash
curl -i http://localhost:8080/api/v1/incidents
```

Перелік доступний без `Authorization`. Для отримання конкретного запису використовується той самий базовий маршрут з його ID у кінці.

#### 10.5. Зміна статусу

```bash
INCIDENT_ID=1 # Замінити фактичним ID зі створеного інциденту.
curl -i -X PATCH "http://localhost:8080/api/v1/incidents/$INCIDENT_ID/status" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"status":"in_progress"}'
curl -i "http://localhost:8080/api/v1/incidents/$INCIDENT_ID"
```

PATCH передає лише новий статус, а наступний GET дозволяє окремо перевірити збережений стан. ID `1` наведено як приклад, а не як гарантовано наявний запис.

#### 10.6. Перевірка відсутнього токена та валідації

```bash
curl -i -X POST http://localhost:8080/api/v1/incidents \
  -H 'Content-Type: application/json' \
  -d '{"title":"Example incident","description":"Example description","priority":"high"}'

curl -i -X PATCH "http://localhost:8080/api/v1/incidents/$INCIDENT_ID/status" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"status":"unknown"}'
```

Перший запит має бути зупинений middleware зі статусом `401`, другий — відхилений валідацією статусу з `400`, якщо JWT дійсний. Аналогічно варто перевірити порожні поля, неправильний email, короткий пароль, змінений підпис і прострочений токен.

#### 10.7. Видалення

```bash
curl -i -X DELETE "http://localhost:8080/api/v1/incidents/$INCIDENT_ID" \
  -H "Authorization: Bearer $TOKEN"
curl -i "http://localhost:8080/api/v1/incidents/$INCIDENT_ID"
```

DELETE є операцією зі зміною даних. Для успішного видалення передбачено `204`, після чого повторне читання має показати відсутність запису. Поточний обробник не перевіряє результат `Delete`, тому повторний GET є важливою ручною перевіркою.

### 11. Результати перевірки та відомі обмеження

Під час підготовки README повторно виконано:

```bash
go test ./...
go vet ./...
go build ./...
```

Команди завершилися успішно. Однак у поточному дереві проєкту **немає файлів `*_test.go`**, і `go test ./...` повідомляє `[no test files]` для пакетів. Отже, це підтверджує компіляцію пакетів, а не проходження автоматизованих сценаріїв авторизації або інтеграційних тестів. Перевірка з реальною PostgreSQL, запуск Docker та ручні HTTP-запити під час підготовки цього README не виконувалися.

За результатами читання актуального коду зафіксовано такі обмеження:

- `Register` ігнорує помилки `GenerateFromPassword` та `db.Create`. Унікальний індекс email є, але помилка дублювання не обробляється окремо.
- `DeleteById` ігнорує помилку видалення; відповідь `204` формується незалежно від результату цієї операції.
- Помилки пошуку інциденту в обробниках читання, оновлення й видалення повертаються як `404` без відокремлення інших збоїв БД. Помилка оновлення статусу повертається як `400`.
- Валідація повертає текст помилки, а не структурований перелік некоректних полів. Назви ключів `error` і `error:` не уніфіковано.
- Для ID у маршрутах не реалізовано окремий розбір і перевірку додатного числового значення.
- Middleware перевіряє токен, але не перевіряє актуальне існування користувача в БД. Відкликання токенів, refresh-токени та перевірка прав власника інциденту не реалізовані.
- `JWT_SECRET` передається через середовище, а DSN поки зберігається в коді; у локальному DSN вимкнено SSL. Публікувати реальні облікові дані не слід.
- Поле `user_id` індексується, але не має задекларованого зовнішнього ключа й обов'язковості на рівні БД.

Ці зауваження описують фактичний стан навчального проєкту та не є твердженням про виконане виправлення. Для промислового використання потрібні додаткові перевірки, тести та налаштування безпеки.

---

## Висновки

- Реалізовано структуру REST API на Go/Gin з окремими моделями, DTO, обробниками та middleware.
- Застосовано теги `binding` і `ShouldBindJSON` для перевірки запитів створення інцидентів, зміни статусу та автентифікації.
- Налаштовано підключення PostgreSQL через GORM і автоматичну міграцію моделей користувача та інциденту.
- Реалізовано bcrypt-перевірку паролів, підписання JWT на 24 години, перевірку Bearer-токенів і прив'язку нового інциденту до авторизованого користувача.
- Підключено Swagger UI та описано основні маршрути декларативними анотаціями. Підтверджено успішну збірку і статичну перевірку поточного коду; інтеграційна перевірка БД та усунення зазначених обмежень залишаються окремими завданнями.

---

# Лістинг

Нижче наведено повний актуальний вміст Go-файлів, `go.mod`, конфігурації Compose та згенерованої документації. Автоматично створені файли Swagger згорнуто для зручності читання. `go.sum` є файлом контрольних сум залежностей, а не вихідним кодом, тому до лістингу його не включено.

**Безпека:** значення пароля в DSN і `POSTGRES_PASSWORD` замінено на `<DB_PASSWORD>` лише у звіті. У виконуваних файлах проєкту ці значення не змінювалися. Інших скорочень у наведених файлах немає.

## main.go

```go
package main

import (
	"log"
	"net/http"
	"os"

	"notion_pr5/database"
	_ "notion_pr5/docs"
	"notion_pr5/handlers"
	"notion_pr5/middleware"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

// @title Incident Management API
// @version 1.0
// @description Incident tracking API with JWT authentication. In Authorize enter: Bearer <token>.
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	secretKey := os.Getenv("JWT_SECRET")
	if len(secretKey) < 32 {
		log.Fatal("JWT_SECRET must contain at least 32 bytes")
	}
	dsn := "host=127.0.0.1 user=pr5 password=<DB_PASSWORD> dbname=pr5 port=5432 sslmode=disable"
	db, err := database.StartDB(dsn)
	if err != nil {
		log.Fatal(err)
	}

	if err := newRouter(db, secretKey).Run(); err != nil {
		log.Fatal(err)
	}
}

func newRouter(db *gorm.DB, secretKey string) *gin.Engine {
	r := gin.Default()
	v1 := r.Group("/api/v1")
	routeForIncident(v1, db, secretKey)
	routeForAuth(v1, db)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	return r
}

func routeForIncident(v1 *gin.RouterGroup, db *gorm.DB, secretKey string) {
	v1.GET("/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "pong"})
	})
	v1.GET("/incidents", func(ctx *gin.Context) {
		handlers.FindAll(ctx, db)
	})
	v1.GET("/incidents/:id", func(ctx *gin.Context) {
		handlers.FindById(ctx, db)
	})
	private := v1.Group("")
	private.Use(middleware.AuthMiddleware(secretKey))
	private.PATCH("/incidents/:id/status", func(ctx *gin.Context) {
		handlers.UpdateById(ctx, db)
	})
	private.POST("/incidents", func(ctx *gin.Context) {
		handlers.CreateIncidentHandler(ctx, db)
	})
	private.DELETE("/incidents/:id", func(ctx *gin.Context) {
		handlers.DeleteById(ctx, db)
	})
}

func routeForAuth(v1 *gin.RouterGroup, db *gorm.DB) {
	v1.POST("/auth/login", func(ctx *gin.Context) {
		handlers.Authorization(ctx, db)
	})
	v1.POST("/auth/register", func(ctx *gin.Context) {
		handlers.Register(ctx, db)
	})
}
```

Файл є точкою входу: перевіряє JWT-секрет, підключає БД, реєструє публічні та захищені маршрути й Swagger UI. Анотації над `main` визначають загальну інформацію про API та схему BearerAuth. Пароль DSN у цьому лістингу замасковано.

## database/config.go

```go
package database

import (
	"notion_pr5/entity"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func StartDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	return db, db.AutoMigrate(&entity.Incident{}, &entity.User{})
}
```

Функція відкриває PostgreSQL-підключення через GORM та виконує `AutoMigrate` для обох моделей. Помилки повертаються виклику, а не приховуються.

## entity/incident.go

```go
package entity

import "time"

type Incident struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"index" json:"user_id"`
	Title       string    `gorm:"size:100;not null" json:"title"`
	Description string    `gorm:"type:text;not null" json:"description"`
	Priority    string    `gorm:"size:20;not null" json:"priority"`
	Status      string    `gorm:"size:20;default:'open'" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}
```

Модель визначає поля інциденту, первинний ключ, індекс автора та початковий статус. Теги `json` задають назви полів відповіді, а `gorm` описують параметри колонок.

## entity/user.go

```go
package entity

import "time"

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Email        string    `gorm:"uniqueIndex;not null"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"CreatedAt"`
}
```

Модель користувача задає унікальність email і зберігає геш пароля. Тег `json:"-"` виключає геш із відповідей API.

## entity/dto/incident_dto.go

```go
package dto

type CreateIncidentDTO struct {
	Title       string `json:"title" binding:"required,min=5,max=100"`
	Description string `json:"description" binding:"required"`
	Priority    string `json:"priority" binding:"required,oneof=low medium high critical"`
}

type UpdateIncidentDTO struct {
	Status string `json:"status" binding:"required,oneof=open in_progress resolved closed"`
}
```

DTO розділяють створення інциденту й зміну статусу. Теги `required`, `min`, `max` та `oneof` перевіряються під час прив’язування JSON.

## entity/dto/user_dto.go

```go
package dto

type RegisterDTO struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginDTO struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}
```

Окремі DTO описують вхідні дані реєстрації та входу. В обох випадках перевіряються формат email та мінімальна довжина пароля.

## auth/claims.go

```go
package auth

import "github.com/golang-jwt/jwt/v5"

// Claims is shared by token creation and verification.
type Claims struct {
	UserID uint `json:"user_id"`
	jwt.RegisteredClaims
}
```

Спільна структура claims поєднує числовий UserID із зареєстрованими JWT-полями. Її використовують і під час видачі, і під час перевірки токенів.

## middleware/auth.go

```go
package middleware

import (
	"net/http"
	"strings"

	"notion_pr5/auth"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func AuthMiddleware(secretKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		unauthorized := func() {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized access"})
		}
		header := c.GetHeader("Authorization")
		if len(secretKey) < 32 || !strings.HasPrefix(header, "Bearer ") {
			unauthorized()
			return
		}

		tokenString := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if tokenString == "" {
			unauthorized()
			return
		}

		claims := &auth.Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
			return []byte(secretKey), nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
		if err != nil || token == nil || !token.Valid || claims.UserID == 0 {
			unauthorized()
			return
		}

		c.Set("userID", claims.UserID)
		c.Next()
	}
}
```

Middleware перевіряє заголовок Bearer, алгоритм, підпис, строк дії та ненульовий ID. При відмові перериває ланцюжок, при успіху передає ID через контекст Gin.

## handlers/incedent_handler.go

```go
package handlers

import (
	"net/http"

	"notion_pr5/entity"
	"notion_pr5/entity/dto"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CreateIncidentHandler godoc
// @Summary Create an incident as the authenticated engineer
// @Tags incidents
// @Accept json
// @Produce json
// @Param request body dto.CreateIncidentDTO true "Incident data"
// @Success 201 {object} entity.Incident
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /incidents [post]
func CreateIncidentHandler(c *gin.Context, db *gorm.DB) {
	value, exists := c.Get("userID")
	userID, valid := value.(uint)
	if !exists || !valid || userID == 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized access"})
		return
	}
	var dto dto.CreateIncidentDTO

	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})

		return
	}
	result := entity.Incident{
		UserID:      userID,
		Title:       dto.Title,
		Description: dto.Description,
		Priority:    dto.Priority,
	}

	if err := db.Create(&result).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create incident"})
		return
	}
	c.JSON(http.StatusCreated, &result)
}

// FindAll godoc
// @Summary List incidents publicly
// @Tags incidents
// @Produce json
// @Success 200 {array} entity.Incident
// @Failure 500 {object} map[string]string
// @Router /incidents [get]
func FindAll(c *gin.Context, db *gorm.DB) {
	var incidents []entity.Incident

	if err := db.Find(&incidents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list incidents"})
		return
	}

	c.JSON(http.StatusOK, &incidents)
}

// FindById godoc
// @Summary Get an incident by ID
// @Tags incidents
// @Produce json
// @Param id path int true "Incident ID"
// @Success 200 {object} entity.Incident
// @Failure 404 {object} map[string]string
// @Router /incidents/{id} [get]
func FindById(ctx *gin.Context, db *gorm.DB) {
	var incident entity.Incident

	id := ctx.Param("id")

	if err := db.First(&incident, id).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, &incident)
}

// UpdateById godoc
// @Summary Update an incident status
// @Tags incidents
// @Accept json
// @Produce json
// @Param id path int true "Incident ID"
// @Param request body dto.UpdateIncidentDTO true "New status"
// @Success 200 {object} entity.Incident
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /incidents/{id}/status [patch]
func UpdateById(ctx *gin.Context, db *gorm.DB) {
	var incident entity.Incident
	var dto dto.UpdateIncidentDTO

	id := ctx.Param("id")

	if err := ctx.ShouldBindJSON(&dto); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if err := db.First(&incident, id).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	err := db.Model(&incident).Update("status", &dto.Status).Error
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, &incident)
}

// DeleteById godoc
// @Summary Delete an incident
// @Tags incidents
// @Produce json
// @Param id path int true "Incident ID"
// @Success 204 "Incident deleted"
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /incidents/{id} [delete]
func DeleteById(ctx *gin.Context, db *gorm.DB) {
	id := ctx.Param("id")

	var incident entity.Incident

	if err := db.First(&incident, id).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	db.Model(&incident).Delete(&incident, id)
	ctx.JSON(http.StatusNoContent, incident)
}
```

Файл містить операції створення, перегляду, зміни статусу й видалення інцидентів та їх Swagger-анотації. Автор створеного запису береться з контексту; особливості обробки помилок описано в підрозділі 11.

## handlers/user_handler.go

```go
package handlers

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"notion_pr5/auth"
	"notion_pr5/entity"
	"notion_pr5/entity/dto"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Register godoc
// @Summary Register a user
// @Tags auth
// @Accept json
// @Produce json
// @Param request body dto.RegisterDTO true "Registration credentials"
// @Success 201 {object} entity.User
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/register [post]
func Register(ctx *gin.Context, db *gorm.DB) {
	var user dto.RegisterDTO

	if err := ctx.ShouldBindJSON(&user); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error:": err.Error()})
		return
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)

	result := entity.User{
		Email:        user.Email,
		PasswordHash: string(hash),
	}

	db.Create(&result)
	ctx.JSON(http.StatusCreated, &result)
}

// Authorization godoc
// @Summary Log in and obtain a JWT valid for 24 hours
// @Tags auth
// @Accept json
// @Produce json
// @Param request body dto.LoginDTO true "Login credentials"
// @Success 200 {object} map[string]string "JWT in the token field"
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/login [post]
func Authorization(ctx *gin.Context, db *gorm.DB) {
	var loginUser dto.LoginDTO
	var user entity.User

	if err := ctx.ShouldBindJSON(&loginUser); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error:": err.Error()})
		return
	}

	if err := db.Where("email = ?", loginUser.Email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Неправильний email або пароль"})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Не вдалося отримати користувача"})
		}
		return
	}

	err := bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash),
		[]byte(loginUser.Password),
	)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Неправильний email або пароль"})
		return
	}

	tokenString, err := createAccessToken(user.ID, []byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Не вдалося створити токен"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"token": tokenString})
}

func createAccessToken(userID uint, secret []byte) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("JWT_SECRET must contain at least 32 bytes")
	}

	now := time.Now()
	claims := auth.Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(uint64(userID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}
```

Файл реалізує реєстрацію, пошук користувача за email, перевірку bcrypt і формування HS256 JWT на 24 години. Відповідь успішного входу містить поле `token`.

## docker-compose.yml

```yaml
services:
  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    environment:
      # Local development defaults only; override for other environments.
      POSTGRES_DB: ${POSTGRES_DB:-pr5}
      POSTGRES_USER: ${POSTGRES_USER:-pr5}
      POSTGRES_PASSWORD: <DB_PASSWORD>
    ports:
      - "127.0.0.1:${POSTGRES_PORT:-5432}:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U \"$$POSTGRES_USER\" -d \"$$POSTGRES_DB\""]
      interval: 5s
      timeout: 5s
      retries: 10
      start_period: 10s

volumes:
  postgres_data:
```

Compose описує PostgreSQL 17 Alpine, локальний порт, змінні ініціалізації, іменований том і healthcheck. Пароль у наведеній копії замасковано; змінні контейнера потрібно узгоджувати з DSN застосунку.

## go.mod

```gomod
module notion_pr5

go 1.27.3

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/gin-gonic/gin v1.12.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/swaggo/files v1.0.1
	github.com/swaggo/gin-swagger v1.6.1
	github.com/swaggo/swag v1.16.6
	golang.org/x/crypto v0.59.0
	gorm.io/driver/postgres v1.6.3
	gorm.io/gorm v1.31.2
)

require (
	github.com/KyleBanks/depth v1.2.1 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic v1.15.0 // indirect
	github.com/bytedance/sonic/loader v0.5.0 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/gabriel-vasile/mimetype v1.4.12 // indirect
	github.com/gin-contrib/sse v1.1.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.6 // indirect
	github.com/go-openapi/spec v0.20.4 // indirect
	github.com/go-openapi/swag v0.19.15 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.1 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.10.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.59.0 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.3.1 // indirect
	go.mongodb.org/mongo-driver/v2 v2.5.0 // indirect
	golang.org/x/arch v0.22.0 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
```

Маніфест визначає модуль `notion_pr5`, версію Go і зафіксовані прямі та непрямі залежності. Наявність `go-sqlmock` у залежностях сама по собі не означає наявності тестів у поточному проєкті.

## docs/docs.go

<details>
<summary>Повний згенерований файл</summary>

```go
// Package docs Code generated by swaggo/swag. DO NOT EDIT
package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {},
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/auth/login": {
            "post": {
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "auth"
                ],
                "summary": "Log in and obtain a JWT valid for 24 hours",
                "parameters": [
                    {
                        "description": "Login credentials",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/dto.LoginDTO"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "JWT in the token field",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            }
        },
        "/auth/register": {
            "post": {
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "auth"
                ],
                "summary": "Register a user",
                "parameters": [
                    {
                        "description": "Registration credentials",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/dto.RegisterDTO"
                        }
                    }
                ],
                "responses": {
                    "201": {
                        "description": "Created",
                        "schema": {
                            "$ref": "#/definitions/entity.User"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            }
        },
        "/incidents": {
            "get": {
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "incidents"
                ],
                "summary": "List incidents publicly",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/entity.Incident"
                            }
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            },
            "post": {
                "security": [
                    {
                        "BearerAuth": []
                    }
                ],
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "incidents"
                ],
                "summary": "Create an incident as the authenticated engineer",
                "parameters": [
                    {
                        "description": "Incident data",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/dto.CreateIncidentDTO"
                        }
                    }
                ],
                "responses": {
                    "201": {
                        "description": "Created",
                        "schema": {
                            "$ref": "#/definitions/entity.Incident"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            }
        },
        "/incidents/{id}": {
            "get": {
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "incidents"
                ],
                "summary": "Get an incident by ID",
                "parameters": [
                    {
                        "type": "integer",
                        "description": "Incident ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/entity.Incident"
                        }
                    },
                    "404": {
                        "description": "Not Found",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            },
            "delete": {
                "security": [
                    {
                        "BearerAuth": []
                    }
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "incidents"
                ],
                "summary": "Delete an incident",
                "parameters": [
                    {
                        "type": "integer",
                        "description": "Incident ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "204": {
                        "description": "Incident deleted"
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "404": {
                        "description": "Not Found",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            }
        },
        "/incidents/{id}/status": {
            "patch": {
                "security": [
                    {
                        "BearerAuth": []
                    }
                ],
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "incidents"
                ],
                "summary": "Update an incident status",
                "parameters": [
                    {
                        "type": "integer",
                        "description": "Incident ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    },
                    {
                        "description": "New status",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/dto.UpdateIncidentDTO"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/entity.Incident"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "404": {
                        "description": "Not Found",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "dto.CreateIncidentDTO": {
            "type": "object",
            "required": [
                "description",
                "priority",
                "title"
            ],
            "properties": {
                "description": {
                    "type": "string"
                },
                "priority": {
                    "type": "string",
                    "enum": [
                        "low",
                        "medium",
                        "high",
                        "critical"
                    ]
                },
                "title": {
                    "type": "string",
                    "maxLength": 100,
                    "minLength": 5
                }
            }
        },
        "dto.LoginDTO": {
            "type": "object",
            "required": [
                "email",
                "password"
            ],
            "properties": {
                "email": {
                    "type": "string"
                },
                "password": {
                    "type": "string",
                    "minLength": 6
                }
            }
        },
        "dto.RegisterDTO": {
            "type": "object",
            "required": [
                "email",
                "password"
            ],
            "properties": {
                "email": {
                    "type": "string"
                },
                "password": {
                    "type": "string",
                    "minLength": 6
                }
            }
        },
        "dto.UpdateIncidentDTO": {
            "type": "object",
            "required": [
                "status"
            ],
            "properties": {
                "status": {
                    "type": "string",
                    "enum": [
                        "open",
                        "in_progress",
                        "resolved",
                        "closed"
                    ]
                }
            }
        },
        "entity.Incident": {
            "type": "object",
            "properties": {
                "created_at": {
                    "type": "string"
                },
                "description": {
                    "type": "string"
                },
                "id": {
                    "type": "integer"
                },
                "priority": {
                    "type": "string"
                },
                "status": {
                    "type": "string"
                },
                "title": {
                    "type": "string"
                },
                "user_id": {
                    "type": "integer"
                }
            }
        },
        "entity.User": {
            "type": "object",
            "properties": {
                "CreatedAt": {
                    "type": "string"
                },
                "email": {
                    "type": "string"
                },
                "id": {
                    "type": "integer"
                }
            }
        }
    },
    "securityDefinitions": {
        "BearerAuth": {
            "type": "apiKey",
            "name": "Authorization",
            "in": "header"
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "1.0",
	Host:             "",
	BasePath:         "/api/v1",
	Schemes:          []string{},
	Title:            "Incident Management API",
	Description:      "Incident tracking API with JWT authentication. In Authorize enter: Bearer <token>.",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
```

Автоматично згенерований Go-файл містить шаблон Swagger-опису та його реєстрацію через пакет swag. Його імпорт у `main.go` робить документацію доступною обробнику gin-swagger.

</details>

## docs/swagger.json

<details>
<summary>Повний згенерований файл</summary>

```json
{
    "swagger": "2.0",
    "info": {
        "description": "Incident tracking API with JWT authentication. In Authorize enter: Bearer \u003ctoken\u003e.",
        "title": "Incident Management API",
        "contact": {},
        "version": "1.0"
    },
    "basePath": "/api/v1",
    "paths": {
        "/auth/login": {
            "post": {
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "auth"
                ],
                "summary": "Log in and obtain a JWT valid for 24 hours",
                "parameters": [
                    {
                        "description": "Login credentials",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/dto.LoginDTO"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "JWT in the token field",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            }
        },
        "/auth/register": {
            "post": {
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "auth"
                ],
                "summary": "Register a user",
                "parameters": [
                    {
                        "description": "Registration credentials",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/dto.RegisterDTO"
                        }
                    }
                ],
                "responses": {
                    "201": {
                        "description": "Created",
                        "schema": {
                            "$ref": "#/definitions/entity.User"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            }
        },
        "/incidents": {
            "get": {
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "incidents"
                ],
                "summary": "List incidents publicly",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/entity.Incident"
                            }
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            },
            "post": {
                "security": [
                    {
                        "BearerAuth": []
                    }
                ],
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "incidents"
                ],
                "summary": "Create an incident as the authenticated engineer",
                "parameters": [
                    {
                        "description": "Incident data",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/dto.CreateIncidentDTO"
                        }
                    }
                ],
                "responses": {
                    "201": {
                        "description": "Created",
                        "schema": {
                            "$ref": "#/definitions/entity.Incident"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            }
        },
        "/incidents/{id}": {
            "get": {
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "incidents"
                ],
                "summary": "Get an incident by ID",
                "parameters": [
                    {
                        "type": "integer",
                        "description": "Incident ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/entity.Incident"
                        }
                    },
                    "404": {
                        "description": "Not Found",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            },
            "delete": {
                "security": [
                    {
                        "BearerAuth": []
                    }
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "incidents"
                ],
                "summary": "Delete an incident",
                "parameters": [
                    {
                        "type": "integer",
                        "description": "Incident ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "204": {
                        "description": "Incident deleted"
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "404": {
                        "description": "Not Found",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            }
        },
        "/incidents/{id}/status": {
            "patch": {
                "security": [
                    {
                        "BearerAuth": []
                    }
                ],
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "incidents"
                ],
                "summary": "Update an incident status",
                "parameters": [
                    {
                        "type": "integer",
                        "description": "Incident ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    },
                    {
                        "description": "New status",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/dto.UpdateIncidentDTO"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/entity.Incident"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "404": {
                        "description": "Not Found",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "dto.CreateIncidentDTO": {
            "type": "object",
            "required": [
                "description",
                "priority",
                "title"
            ],
            "properties": {
                "description": {
                    "type": "string"
                },
                "priority": {
                    "type": "string",
                    "enum": [
                        "low",
                        "medium",
                        "high",
                        "critical"
                    ]
                },
                "title": {
                    "type": "string",
                    "maxLength": 100,
                    "minLength": 5
                }
            }
        },
        "dto.LoginDTO": {
            "type": "object",
            "required": [
                "email",
                "password"
            ],
            "properties": {
                "email": {
                    "type": "string"
                },
                "password": {
                    "type": "string",
                    "minLength": 6
                }
            }
        },
        "dto.RegisterDTO": {
            "type": "object",
            "required": [
                "email",
                "password"
            ],
            "properties": {
                "email": {
                    "type": "string"
                },
                "password": {
                    "type": "string",
                    "minLength": 6
                }
            }
        },
        "dto.UpdateIncidentDTO": {
            "type": "object",
            "required": [
                "status"
            ],
            "properties": {
                "status": {
                    "type": "string",
                    "enum": [
                        "open",
                        "in_progress",
                        "resolved",
                        "closed"
                    ]
                }
            }
        },
        "entity.Incident": {
            "type": "object",
            "properties": {
                "created_at": {
                    "type": "string"
                },
                "description": {
                    "type": "string"
                },
                "id": {
                    "type": "integer"
                },
                "priority": {
                    "type": "string"
                },
                "status": {
                    "type": "string"
                },
                "title": {
                    "type": "string"
                },
                "user_id": {
                    "type": "integer"
                }
            }
        },
        "entity.User": {
            "type": "object",
            "properties": {
                "CreatedAt": {
                    "type": "string"
                },
                "email": {
                    "type": "string"
                },
                "id": {
                    "type": "integer"
                }
            }
        }
    },
    "securityDefinitions": {
        "BearerAuth": {
            "type": "apiKey",
            "name": "Authorization",
            "in": "header"
        }
    }
}
```

Згенерований JSON описує схеми, параметри, відповіді та вимоги авторизації операцій API. Після зміни анотацій файл потрібно створити повторно генератором.

</details>

## docs/swagger.yaml

<details>
<summary>Повний згенерований файл</summary>

```yaml
basePath: /api/v1
definitions:
  dto.CreateIncidentDTO:
    properties:
      description:
        type: string
      priority:
        enum:
        - low
        - medium
        - high
        - critical
        type: string
      title:
        maxLength: 100
        minLength: 5
        type: string
    required:
    - description
    - priority
    - title
    type: object
  dto.LoginDTO:
    properties:
      email:
        type: string
      password:
        minLength: 6
        type: string
    required:
    - email
    - password
    type: object
  dto.RegisterDTO:
    properties:
      email:
        type: string
      password:
        minLength: 6
        type: string
    required:
    - email
    - password
    type: object
  dto.UpdateIncidentDTO:
    properties:
      status:
        enum:
        - open
        - in_progress
        - resolved
        - closed
        type: string
    required:
    - status
    type: object
  entity.Incident:
    properties:
      created_at:
        type: string
      description:
        type: string
      id:
        type: integer
      priority:
        type: string
      status:
        type: string
      title:
        type: string
      user_id:
        type: integer
    type: object
  entity.User:
    properties:
      CreatedAt:
        type: string
      email:
        type: string
      id:
        type: integer
    type: object
info:
  contact: {}
  description: 'Incident tracking API with JWT authentication. In Authorize enter:
    Bearer <token>.'
  title: Incident Management API
  version: "1.0"
paths:
  /auth/login:
    post:
      consumes:
      - application/json
      parameters:
      - description: Login credentials
        in: body
        name: request
        required: true
        schema:
          $ref: '#/definitions/dto.LoginDTO'
      produces:
      - application/json
      responses:
        "200":
          description: JWT in the token field
          schema:
            additionalProperties:
              type: string
            type: object
        "400":
          description: Bad Request
          schema:
            additionalProperties:
              type: string
            type: object
        "401":
          description: Unauthorized
          schema:
            additionalProperties:
              type: string
            type: object
        "500":
          description: Internal Server Error
          schema:
            additionalProperties:
              type: string
            type: object
      summary: Log in and obtain a JWT valid for 24 hours
      tags:
      - auth
  /auth/register:
    post:
      consumes:
      - application/json
      parameters:
      - description: Registration credentials
        in: body
        name: request
        required: true
        schema:
          $ref: '#/definitions/dto.RegisterDTO'
      produces:
      - application/json
      responses:
        "201":
          description: Created
          schema:
            $ref: '#/definitions/entity.User'
        "400":
          description: Bad Request
          schema:
            additionalProperties:
              type: string
            type: object
        "500":
          description: Internal Server Error
          schema:
            additionalProperties:
              type: string
            type: object
      summary: Register a user
      tags:
      - auth
  /incidents:
    get:
      produces:
      - application/json
      responses:
        "200":
          description: OK
          schema:
            items:
              $ref: '#/definitions/entity.Incident'
            type: array
        "500":
          description: Internal Server Error
          schema:
            additionalProperties:
              type: string
            type: object
      summary: List incidents publicly
      tags:
      - incidents
    post:
      consumes:
      - application/json
      parameters:
      - description: Incident data
        in: body
        name: request
        required: true
        schema:
          $ref: '#/definitions/dto.CreateIncidentDTO'
      produces:
      - application/json
      responses:
        "201":
          description: Created
          schema:
            $ref: '#/definitions/entity.Incident'
        "400":
          description: Bad Request
          schema:
            additionalProperties:
              type: string
            type: object
        "401":
          description: Unauthorized
          schema:
            additionalProperties:
              type: string
            type: object
        "500":
          description: Internal Server Error
          schema:
            additionalProperties:
              type: string
            type: object
      security:
      - BearerAuth: []
      summary: Create an incident as the authenticated engineer
      tags:
      - incidents
  /incidents/{id}:
    delete:
      parameters:
      - description: Incident ID
        in: path
        name: id
        required: true
        type: integer
      produces:
      - application/json
      responses:
        "204":
          description: Incident deleted
        "401":
          description: Unauthorized
          schema:
            additionalProperties:
              type: string
            type: object
        "404":
          description: Not Found
          schema:
            additionalProperties:
              type: string
            type: object
      security:
      - BearerAuth: []
      summary: Delete an incident
      tags:
      - incidents
    get:
      parameters:
      - description: Incident ID
        in: path
        name: id
        required: true
        type: integer
      produces:
      - application/json
      responses:
        "200":
          description: OK
          schema:
            $ref: '#/definitions/entity.Incident'
        "404":
          description: Not Found
          schema:
            additionalProperties:
              type: string
            type: object
      summary: Get an incident by ID
      tags:
      - incidents
  /incidents/{id}/status:
    patch:
      consumes:
      - application/json
      parameters:
      - description: Incident ID
        in: path
        name: id
        required: true
        type: integer
      - description: New status
        in: body
        name: request
        required: true
        schema:
          $ref: '#/definitions/dto.UpdateIncidentDTO'
      produces:
      - application/json
      responses:
        "200":
          description: OK
          schema:
            $ref: '#/definitions/entity.Incident'
        "400":
          description: Bad Request
          schema:
            additionalProperties:
              type: string
            type: object
        "401":
          description: Unauthorized
          schema:
            additionalProperties:
              type: string
            type: object
        "404":
          description: Not Found
          schema:
            additionalProperties:
              type: string
            type: object
      security:
      - BearerAuth: []
      summary: Update an incident status
      tags:
      - incidents
securityDefinitions:
  BearerAuth:
    in: header
    name: Authorization
    type: apiKey
swagger: "2.0"
```

Це YAML-подання тієї самої Swagger-специфікації. Воно створюється разом із JSON та не є окремою реалізацією маршрутизації чи перевірки доступу.

</details>

