# CLAUDE.md

이 파일은 이 저장소(db-lite)에서 작업하는 Claude 세션을 위한 지침입니다. 도메인 용어는 [CONTEXT.md](CONTEXT.md), 사용자 대상 안내는 [README.md](README.md), 아키텍처 결정 배경은 [docs/adr](docs/adr)를 참고하세요.

## 프로젝트 구조

- `server/`: Go 백엔드 (REST API + `go:embed`로 내장한 React 클라이언트를 함께 서빙)
- `client/`: React + Vite 프론트엔드, CodeMirror 6 기반 쿼리 에디터
- `electron/`: 서버를 내장하지 않는 얇은 데스크톱 셸 (ADR 0010) — 서버 주소 입력 후 그 주소를 BrowserWindow로 띄울 뿐, client/의 코드는 번들링하지 않는다
- 개발 모드에서는 `server`(Go, 기본 :8080)와 `client`(Vite dev 서버, 기본 :5173)를 따로 띄운다 — README의 "개발 모드" 절 참고

## 이 환경의 개발 습관

- **Go가 기본 PATH에 없다.** 셸에서 `go` 명령을 쓰기 전에 `export PATH="/c/Program Files/Go/bin:$PATH"`(bash) 또는 동등한 PowerShell 설정이 필요할 수 있다.
- **서버 재시작**: `DBTOOL_SQLITE_PATH=dev.sqlite DBTOOL_JWT_SECRET=<아무값> ./server.exe` — `DBTOOL_JWT_SECRET`은 필수(없으면 기동 자체를 거부한다), `DBTOOL_SQLITE_PATH`를 안 주면 매번 새 `dbtool.sqlite`가 생겨 기존 데이터(admin 계정 등)가 안 보이는 것처럼 보일 수 있다.
- **PowerShell/cmd/bash 문법이 다르다.** 사용자가 겪었듯 `KEY=value \` 형식은 PowerShell/cmd에서 안 먹힌다. 명령어를 안내할 때 셸별로 구분해서 주거나, `.env` 파일 사용을 권한다(`server/.env.example`, `client/.env.example` 참고).
- **`server/internal/httpapi/dist/.gitkeep`이 자주 실수로 사라진다.** `npm run build` 후 `dist`를 이 경로로 복사하는 과정에서 디렉터리를 통째로 지웠다 다시 채우면 `.gitkeep`이 같이 날아간다 — 커밋 전에 `git status`로 꼭 확인한다. 이 파일이 없어도 `go build`는 되지만(임베드 대상이 비어있을 뿐), 저장소에는 항상 있어야 신규 클론 시 클라이언트 빌드 없이도 서버가 컴파일된다.

## 기능 검증 원칙

- **코드만 보고 "될 것 같다"로 끝내지 않는다.** 이 저장소는 DB 도구라 실제 버그(예: MySQL 비밀번호 특수문자로 DSN이 깨지는 문제, Postgres가 항상 고정 DB에만 붙던 문제, Oracle 서비스명 누락, MySQL이 모든 테이블의 PK 제약조건 이름을 "PRIMARY"로 고정해서 이름만으로 조인하면 다른 테이블 PK가 섞여 들어오는 문제)가 전부 실제 Docker 컨테이너로 조회/쓰기까지 돌려봐야 드러났다. DB 연결이 얽힌 변경은 Docker로 해당 DB 종류를 띄워 Connection 등록 → Permission 부여 → 쿼리 실행까지 브라우저(Playwright)로 직접 확인한다.
- **기존 기능을 일반화하는 변경은, 그 일반화가 원래 목표했던 가장 단순한 형태부터 반드시 실제로 돌려본다.** ADR 0011(JOIN 셀 다운로드) 작업 때 `SELECT u.id, u.name FROM ...` 같은 명시적 컬럼 목록으로는 여러 번 Docker 검증을 했지만, 정작 원래 지원하려던 목표였던 `SELECT * FROM a JOIN b`(와일드카드)는 한 번도 실행해보지 않아 배포 후에야 회귀(컬럼이 1개로 잘림)를 사용자가 발견했다. "인접한 이미 되는 케이스"만 반복 검증하고 "이번에 새로 되게 하려는 그 케이스"를 빠뜨리기 쉽다.
- 프론트엔드 전용 변경(에디터 UI, 자동완성 등)은 Go 서버 재빌드가 필요 없다 — 헷갈리지 말 것.
- 테스트에 쓴 Docker 컨테이너, Connection, Permission은 세션이 끝나기 전에 정리한다(`docker rm -f`, 테스트용 Connection 삭제).
- **Electron(`electron/`) 변경은 Playwright로 볼 수 없다.** `cd electron && npm start`로 띄운 뒤, `tasklist //FI "IMAGENAME eq electron.exe" //V`(빌드 후엔 `DB Lite.exe`)나 PowerShell `Get-Process -Name "<이름>" | Select MainWindowTitle`로 창 제목을 읽어 실제로 원하는 화면(설정 창 vs 서버 접속 화면)이 떴는지 확인한다. 끝나면 `taskkill //F //IM electron.exe //T`로 정리.
- **Electron 패키징 검증**: `npm run build`(electron-builder)로 만든 `dist/win-unpacked/*.exe`를 실행해서 확인하고, `npx asar list dist/win-unpacked/resources/app.asar`로 의도한 파일만 들어갔는지 점검한다. 설치 프로그램(`*.exe` installer) 자체는 시스템을 변경하므로 실행하지 않는다.
- **`electron/`의 userData(설정 저장 위치)는 `package.json`의 `name` 필드(`db-lite-desktop`) 기준으로 생성된다** (`productName`이 아님) — dev(`npm start`)와 패키징 빌드가 같은 설정 파일을 공유하므로, 이전 테스트에서 저장된 서버 주소가 다음 실행에도 그대로 남아있을 수 있다.

## Go 테스트 컨벤션

- `server/internal/store`: `:memory:` SQLite로 격리된 테스트. `newTestStore(t)` 헬퍼 사용.
- `server/internal/dbconn`: DSN 생성 로직은 순수 함수 테스트(`driverAndDSN`), 실제 드라이버로 재파싱해서 host/password가 원래 값으로 복원되는지까지 검증(`TestDriverAndDSN_PasswordWithSpecialCharacters` 참고).
- 새 버그를 고칠 때는 그 버그를 정확히 재현하는 회귀 테스트를 먼저(또는 같이) 추가한다.

## 커밋/PR 관행

- 커밋 메시지는 한국어로 작성하고, 무엇을 왜 고쳤는지와 **어떻게 검증했는지**(예: "Docker MySQL로 실제 조회/쓰기 테스트")를 구체적으로 남긴다.
- 사용자가 명시적으로 요청할 때만 커밋한다. 커밋 후에는 `git push`하고 GitHub Actions 결과(`gh run list`)를 확인할 때까지가 한 세트다.
- 논리적으로 분리되는 변경은 커밋도 나눈다(예: 버그 수정과 정리 작업을 분리).

## 진행 상황 추적

- 오픈소스 공개 준비 관련 결정/작업은 wayfinder 맵([DB Lite 오픈소스 공개 준비](https://github.com/tokkaiiii/db-lite/issues/1))에 기록돼 있다. 이 저장소는 이미 GitHub Issues를 wayfinder 트래커로 쓰고 있다.
- 이 맵은 wayfinder 기본값(순수 결정 목록)과 달리, 실행(작업) 티켓까지 포함하는 백로그로 의도적으로 운영 중이다.
