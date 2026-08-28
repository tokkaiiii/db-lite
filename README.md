# DB Lite

사내망 등 폐쇄망에서 MSSQL/MySQL/PostgreSQL/Oracle DB를 브라우저로 조회·실행할 수 있게 해주는 가벼운 셀프호스팅 도구입니다. Go 서버 하나가 DB 접근 권한이 있는 위치에서 상시 구동되고, 사용자는 브라우저로 접속해 사용합니다.

## 왜 만들었나

Docker나 JVM 설치조차 어려운 폐쇄망 Windows 서버에도 띄울 수 있는 "DBeaver-lite" 같은 도구가 필요했습니다. 그래서:

- CloudBeaver 같은 기존 웹 도구 대신, 실행 파일 하나로 배포 가능한 자체 Go 서버로 구현했습니다.
- MSSQL/MySQL/PostgreSQL/Oracle 드라이버 전부 순수 Go 구현체를 사용합니다 (Oracle도 godror 대신 go-ora — Oracle Instant Client 설치가 필요 없습니다). 설치 부담을 피하는 것이 이 프로젝트의 시작 동기이기 때문입니다.

배경과 설계 결정은 [docs/adr](docs/adr)와 [CONTEXT.md](CONTEXT.md)에 더 자세히 정리되어 있습니다.

## 핵심 개념

- **User**: 이 앱에 자체 ID/PW로 로그인하는 계정. 실제 DB 계정과는 별개이며, 그 자체로는 아무 DB 접근 권한도 없습니다.
- **Admin**: Connection을 등록/삭제하고 User에게 Permission을 부여/회수하는 상위 역할.
- **Connection**: 하나의 DB 서버 인스턴스(호스트+포트+DB 종류+공유 계정)를 가리키는 등록 단위. Permission을 부여하는 최소 단위입니다.
- **Permission**: User와 Connection 사이의 접근 수준 — `없음`(안 보임) / `읽기`(SELECT만) / `쓰기`(모든 SQL 실행 가능).
- **Audit Log**: Write Query(조회성이 아닌 모든 구문) 시도를 기록하는 로그. 거부된 시도도 포함하며, 조회(SELECT)는 기록하지 않습니다.

자세한 용어 정의는 [CONTEXT.md](CONTEXT.md)를 참고하세요.

## 실행 방법

### 요구 사항

- Go 1.26+
- Node.js 18+ (클라이언트 빌드용)

### 1. 클라이언트 빌드 후 서버에 내장

서버 바이너리 하나로 API와 웹 UI를 함께 서빙합니다. 클라이언트를 빌드해 서버가 내장(`go:embed`)하는 위치에 복사한 뒤 빌드하세요.

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

### 2. 환경변수 설정 후 실행

| 환경변수 | 필수 | 기본값 | 설명 |
|---|---|---|---|
| `DBTOOL_JWT_SECRET` | **예** | 없음 (미설정 시 기동 거부) | 로그인 세션(JWT) 서명 키. 충분히 무작위한 문자열로 직접 설정해야 합니다 |
| `DBTOOL_LISTEN_ADDR` | 아니오 | `:8080` | 서버가 바인딩할 주소 |
| `DBTOOL_SQLITE_PATH` | 아니오 | `dbtool.sqlite` | 앱 메타데이터(User/Connection/Permission/Audit Log)를 저장할 SQLite 파일 경로 |
| `DBTOOL_BOOTSTRAP_ADMIN_USER` / `DBTOOL_BOOTSTRAP_ADMIN_PASSWORD` | 아니오 | 없음 | 최초 실행 시(User가 0명일 때만) 이 계정으로 첫 Admin을 자동 생성 |

```bash
DBTOOL_JWT_SECRET="충분히-무작위한-값으로-바꾸세요" \
DBTOOL_BOOTSTRAP_ADMIN_USER=admin \
DBTOOL_BOOTSTRAP_ADMIN_PASSWORD="바꾸세요" \
./dbtool
```

실행 후 `http://<서버주소>:8080`으로 접속해 방금 만든 관리자 계정으로 로그인하면, Connection 등록과 User별 Permission 부여를 시작할 수 있습니다.

### 개발 모드

프론트엔드를 수정하며 핫 리로드가 필요하면 클라이언트 dev 서버와 Go 서버를 따로 띄우면 됩니다 (`client/vite.config.ts`가 `/api`를 `localhost:8080`으로 프록시합니다).

```bash
# 터미널 1
cd server && DBTOOL_JWT_SECRET=dev-secret DBTOOL_SQLITE_PATH=dev.sqlite go run ./cmd/server

# 터미널 2
cd client && npm run dev
```

## 지원 DB

MSSQL, MySQL, PostgreSQL, Oracle. 각 Connection은 개별 DB 서버 인스턴스 단위로 등록하며, 그 안의 특정 데이터베이스(카탈로그)나 테이블 단위 권한 세분화는 아직 지원하지 않습니다.

## 보안 관련 주의사항

셀프호스팅 도구로서 알아두어야 할 현재 상태입니다:

- **Connection 비밀번호는 SQLite에 평문으로 저장됩니다.** 아직 at-rest 암호화가 구현되어 있지 않으므로, SQLite 파일(`DBTOOL_SQLITE_PATH`)의 파일시스템 접근 권한을 반드시 제한하세요. 암호화 지원은 이후 이슈로 계획 중입니다.
- `DBTOOL_JWT_SECRET`을 반드시 직접 설정하세요. 미설정 시 서버는 기동을 거부합니다 — 하드코딩된 기본값으로 조용히 뜨는 일은 없습니다.
- 모든 User가 Connection에 접속하는 DB 계정은 앱이 관리하는 **공유 계정** 하나입니다. User별 접근 통제와 쓰기 이력 추적은 전적으로 이 앱(Permission, Audit Log)이 책임지며, DB 서버 자체의 로그만으로는 어떤 User가 실행한 쿼리인지 알 수 없습니다.

## 라이선스

[MIT](LICENSE)
