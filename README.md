# DB Lite

사내망 등 폐쇄망에서 MSSQL/MySQL/PostgreSQL/Oracle DB를 브라우저로 조회·실행할 수 있게 해주는 가벼운 셀프호스팅 도구입니다. Go 서버 하나가 DB 접근 권한이 있는 위치에서 상시 구동되고, 사용자는 브라우저 또는 Electron 클라이언트로 접속해 사용합니다.

## 왜 만들었나

Docker나 JVM 설치조차 어려운 폐쇄망 Windows 서버에도 띄울 수 있는 "DBeaver-lite" 같은 도구가 필요했습니다. 그래서:

- CloudBeaver 같은 기존 웹 도구 대신, 실행 파일 하나로 배포 가능한 자체 Go 서버로 구현했습니다.
- MSSQL/MySQL/PostgreSQL/Oracle 드라이버 전부 순수 Go 구현체를 사용합니다 (Oracle도 godror 대신 go-ora — Oracle Instant Client 설치가 필요 없습니다). 설치 부담을 피하는 것이 이 프로젝트의 시작 동기이기 때문입니다.

배경과 설계 결정은 [docs/adr](docs/adr)와 [CONTEXT.md](CONTEXT.md)에 더 자세히 정리되어 있습니다.

## 핵심 개념

- **User**: 이 앱에 자체 ID/PW로 로그인하는 계정. 실제 DB 계정과는 별개이며, 그 자체로는 아무 DB 접근 권한도 없습니다.
- **Admin**: Connection을 등록/삭제하고 User에게 Permission을 부여/회수하는 상위 역할.
- **Connection**: 하나의 DB 서버 인스턴스(호스트+포트+DB 종류+공유 계정)를 가리키는 등록 단위. Permission을 부여하는 최소 단위입니다.
- **Catalog**: Connection 하나가 가리키는 서버 인스턴스 안에서 사용자가 화면에서 고르는 개별 데이터베이스. Permission과 무관한 순수 연결 대상 선택입니다.
- **Permission**: User와 Connection 사이의 접근 수준 — `없음`(안 보임) / `읽기`(SELECT만) / `쓰기`(모든 SQL 실행 가능).
- **Audit Log**: Write Query(조회성이 아닌 모든 구문) 시도를 기록하는 로그. 거부된 시도도 포함하며, 조회(SELECT)는 기록하지 않습니다.

자세한 용어 정의는 [CONTEXT.md](CONTEXT.md)를 참고하세요.

## 실행 방법

### 요구 사항

- Go 1.26+
- Node.js 18+ (클라이언트 빌드용)

### 1. 클라이언트 빌드 후 서버에 내장

서버 바이너리 하나로 API와 웹 UI를 함께 서빙합니다. 클라이언트를 빌드해 서버가 내장(`go:embed`)하는 위치에 복사한 뒤 빌드하세요.

macOS/Linux:

```bash
cd client
npm install
npm run build

# 빌드 산출물을 서버가 embed하는 위치로 복사
rm -rf ../server/internal/httpapi/dist
cp -r dist ../server/internal/httpapi/dist

cd ../server
go build -o dbtool ./cmd/server
```

Windows (PowerShell):

```powershell
cd client
npm install
npm run build

# 빌드 산출물을 서버가 embed하는 위치로 복사
Remove-Item -Recurse -Force ..\server\internal\httpapi\dist -ErrorAction SilentlyContinue
Copy-Item -Recurse dist ..\server\internal\httpapi\dist

cd ..\server
go build -o dbtool.exe ./cmd/server
```

### 2. 환경변수 설정 후 실행

| 환경변수 | 필수 | 기본값 | 설명 |
|---|---|---|---|
| `DBTOOL_JWT_SECRET` | **예** | 없음 (미설정 시 기동 거부) | 로그인 세션(JWT) 서명 키. 충분히 무작위한 문자열로 직접 설정해야 합니다 |
| `DBTOOL_LISTEN_ADDR` | 아니오 | `:8080` | 서버가 바인딩할 주소 |
| `DBTOOL_SQLITE_PATH` | 아니오 | `dbtool.sqlite` | 앱 메타데이터(User/Connection/Permission/Audit Log)를 저장할 SQLite 파일 경로 |
| `DBTOOL_BOOTSTRAP_ADMIN_USER` / `DBTOOL_BOOTSTRAP_ADMIN_PASSWORD` | 아니오 | 없음 | 최초 실행 시(User가 0명일 때만) 이 계정으로 첫 Admin을 자동 생성 |
| `DBTOOL_ENV_FILE` | 아니오 | `.env` | 아래 `.env` 파일을 읽어올 경로 |

macOS/Linux (bash/zsh):

```bash
DBTOOL_JWT_SECRET="충분히-무작위한-값으로-바꾸세요" \
DBTOOL_BOOTSTRAP_ADMIN_USER=admin \
DBTOOL_BOOTSTRAP_ADMIN_PASSWORD="바꾸세요" \
./dbtool
```

Windows (PowerShell) — `KEY=value \` 형식은 PowerShell 문법이 아니므로 `$env:`로 하나씩 설정하세요:

```powershell
$env:DBTOOL_JWT_SECRET="충분히-무작위한-값으로-바꾸세요"
$env:DBTOOL_BOOTSTRAP_ADMIN_USER="admin"
$env:DBTOOL_BOOTSTRAP_ADMIN_PASSWORD="바꾸세요"
./dbtool.exe
```

Windows (cmd):

```cmd
set DBTOOL_JWT_SECRET=충분히-무작위한-값으로-바꾸세요
set DBTOOL_BOOTSTRAP_ADMIN_USER=admin
set DBTOOL_BOOTSTRAP_ADMIN_PASSWORD=바꾸세요
dbtool.exe
```

셸마다 문법이 달라 번거롭다면, 환경변수 대신 **`.env` 파일**을 쓸 수 있습니다. **현재 작업 디렉터리(실행 시 `cd`한 위치, 바이너리 파일 위치가 아닙니다)**에 `.env` 파일을 두면 실행 시 자동으로 읽습니다(`server/.env.example` 참고). 다른 위치에서 절대경로로 실행하거나 Windows 서비스로 등록하는 등 작업 디렉터리가 바이너리 위치와 다르면 `.env`를 못 찾을 수 있으니, 이런 경우 `DBTOOL_ENV_FILE`로 경로를 직접 지정하세요. 이미 설정된 실제 환경변수가 있으면 그 값이 항상 우선합니다.

```
# .env
DBTOOL_JWT_SECRET=충분히-무작위한-값으로-바꾸세요
DBTOOL_BOOTSTRAP_ADMIN_USER=admin
DBTOOL_BOOTSTRAP_ADMIN_PASSWORD=바꾸세요
```

```bash
./dbtool   # 셸 상관없이 .env를 읽어 그대로 실행
```

`.env`에는 비밀번호가 평문으로 남으므로, 파일 접근 권한을 제한하고 절대 커밋하지 마세요(`.gitignore`에 이미 `.env`가 등록되어 있습니다).

실행 후 `http://<서버주소>:8080`으로 접속해 방금 만든 관리자 계정으로 로그인하면, Connection 등록과 User별 Permission 부여를 시작할 수 있습니다.

### 개발 모드

프론트엔드를 수정하며 핫 리로드가 필요하면 클라이언트 dev 서버와 Go 서버를 따로 띄우면 됩니다 (`client/vite.config.ts`가 기본적으로 `/api`를 `localhost:8080`으로 프록시합니다).

서버를 `DBTOOL_LISTEN_ADDR`로 다른 포트에 띄웠거나 클라이언트 dev 서버 포트(기본 `5173`)를 바꾸고 싶다면, `client/.env.example`을 `client/.env`로 복사해 값을 채우세요:

```
# client/.env
VITE_DEV_PORT=5174
VITE_API_PROXY_TARGET=http://localhost:9090
```

macOS/Linux:

```bash
# 터미널 1 (포트를 바꾸지 않는다면 DBTOOL_LISTEN_ADDR는 생략 가능)
cd server && DBTOOL_JWT_SECRET=dev-secret DBTOOL_SQLITE_PATH=dev.sqlite DBTOOL_LISTEN_ADDR=:9090 go run ./cmd/server

# 터미널 2
cd client && npm run dev
```

Windows (PowerShell):

```powershell
# 터미널 1 (포트를 바꾸지 않는다면 DBTOOL_LISTEN_ADDR는 생략 가능)
cd server
$env:DBTOOL_JWT_SECRET="dev-secret"; $env:DBTOOL_SQLITE_PATH="dev.sqlite"; $env:DBTOOL_LISTEN_ADDR=":9090"; go run ./cmd/server

# 터미널 2
cd client
npm run dev
```

### (선택) Electron 클라이언트로 접속

브라우저 대신 데스크톱 프로그램 형태로 접속하고 싶다면 `electron/`의 얇은 클라이언트를 쓸 수 있습니다. 서버를 내장하지 않고 이미 떠 있는 서버에 접속만 하는 셸입니다 ([ADR 0010](docs/adr/0010-electron-thin-client-shell.md), [CONTEXT.md](CONTEXT.md) 참고).

```bash
cd electron
npm install
npm start   # 처음 실행이면 서버 주소를 입력하는 창이 뜹니다
```

Windows용 설치 파일을 만들려면 `npm run build`(electron-builder)를 실행하세요. `electron/dist/`에 결과물이 생성됩니다.

## 주요 기능

- **쿼리 실행 화면 SQL 자동완성**: 키워드, 테이블명, `별칭.컬럼` 자동완성을 모두 지원합니다. 테이블을 자동완성으로 선택하면 `테이블명 별칭` 형태로 자동 삽입되고(예: `employees e`), 그 별칭으로 컬럼 자동완성도 바로 됩니다. Tab으로 자동완성 확정, Ctrl/Cmd+Enter로 커서가 속한 문장(또는 선택 영역)만 실행합니다.
- **포맷팅 / 문장 완성**: JetBrains DataGrip과 동일한 단축키 — `Ctrl/Cmd+Alt+L`로 포맷팅, `Ctrl/Cmd+Shift+Enter`로 현재 문장 끝에 세미콜론 자동 추가.
- **Catalog 선택**: MySQL/Postgres/MSSQL은 하나의 Connection 안에 여러 데이터베이스가 있을 수 있어, 쿼리 실행 화면에서 어느 데이터베이스에 붙을지 드롭다운으로 고를 수 있습니다(`?catalog=` URL 파라미터로 유지). Oracle은 Connection의 서비스명이 이미 대상을 고정하므로 이 드롭다운이 뜨지 않습니다.
- **Connection 수정**: 등록 후 오타를 발견해도 삭제 없이 바로 고칠 수 있습니다(그 Connection에 걸린 Permission이 그대로 유지됩니다). 비밀번호 입력란을 비워두면 기존 비밀번호가 유지됩니다.
- **권한 현황 조회**: Permissions 화면에서 현재 누가 어떤 Connection에 어떤 권한을 가졌는지 목록으로 보고, 바로 회수할 수 있습니다.
- **결과 행/셀 크기 제한**: SELECT 결과는 최대 1,000행까지만 내려받고, 그 이상은 잘렸다는 표시가 함께 옵니다. BLOB/XML처럼 큰 셀 값은 서버에서 2KB로 잘라 렌더링 지연을 줄입니다.
- **셀 원본 값 다운로드**: PK가 있는 단일 테이블 `SELECT *` 조회 결과뿐 아니라, JOIN·1단계 서브쿼리·`SELECT *`(혼합 와일드카드 포함) 결과에서도 컬럼별로 출처 테이블을 추적할 수 있으면 원본 값을 그대로 파일로 내려받을 수 있습니다(다운로드 버튼이 컬럼 단위로 뜹니다). CTE(`WITH`)·UNION은 아직 지원하지 않습니다.

## 지원 DB

MSSQL, MySQL, PostgreSQL, Oracle. 각 Connection은 개별 DB 서버 인스턴스 단위로 등록합니다. MySQL/Postgres/MSSQL은 쿼리 실행 화면에서 Catalog(개별 데이터베이스)를 선택할 수 있고, Oracle은 Connection 등록 시 입력한 서비스명/SID로 대상이 고정됩니다. 테이블 단위 권한 세분화는 아직 지원하지 않습니다.

## 보안 관련 주의사항

셀프호스팅 도구로서 알아두어야 할 현재 상태입니다:

- **Connection 비밀번호는 SQLite에 평문으로 저장됩니다.** 아직 at-rest 암호화가 구현되어 있지 않으므로, SQLite 파일(`DBTOOL_SQLITE_PATH`)의 파일시스템 접근 권한을 반드시 제한하세요. 암호화 지원은 이후 이슈로 계획 중입니다.
- `DBTOOL_JWT_SECRET`을 반드시 직접 설정하세요. 미설정 시 서버는 기동을 거부합니다 — 하드코딩된 기본값으로 조용히 뜨는 일은 없습니다.
- 모든 User가 Connection에 접속하는 DB 계정은 앱이 관리하는 **공유 계정** 하나입니다. User별 접근 통제와 쓰기 이력 추적은 전적으로 이 앱(Permission, Audit Log)이 책임지며, DB 서버 자체의 로그만으로는 어떤 User가 실행한 쿼리인지 알 수 없습니다.

## 라이선스

[MIT](LICENSE)
