# ansm

나씀(NSSM, the non-sucking service manager)의 Go 이식본.

임의의 프로그램을 Windows 서비스로 등록해 대신 실행하고, 죽으면 다시 살려준다.
**밖에서 보이는 동작은 원본과 같게 두는 것**이 이식의 목표다. 명령·설정 값 이름·
저장 위치·종료 코드·이벤트 번호를 그대로 승계하므로 기존 설치본을 이어받을 수 있다.

- 대상 플랫폼: Windows 전용
- 설계: `ansm.default.0001` 그룹의 D0006(기본설계) · P0007(프로토콜설계) · L0008(로직설계)

## 빌드와 시험

```
go build ./cmd/ansm
go test ./...
go vet ./...
```

외부 의존성이 없다. cgo 도 쓰지 않는다. 리소스 컴파일러(`mc.exe`·`rc.exe`·
`windres`)도 쓰지 않는다 — `cmd/ansm/rsrc_windows_*.syso` 가 저장소에 함께
들어 있어서 평소 빌드는 위 한 줄로 끝난다.

메시지 목록(`resources/messages.mc`)이나 아이콘을 고쳤을 때만 다시 만든다.

```
go generate ./cmd/ansm
```

배포 산출물(64비트·32비트)은 아래로 만든다. 버전과 빌드 일자를 저장소
이력에서 뽑아 링커로 주입하므로, 같은 커밋에서는 몇 번을 돌려도 같은
바이트가 나온다.

```
pwsh tools\dist.ps1
```

## 구성

| 경로 | 내용 |
|---|---|
| `cmd/ansm` | 진입점 |
| `internal/app` | 실행 모드 판별과 관리 명령 처리 (L0008 2.1·4.1, P0007 2·3장) |
| `internal/cli` | 명령 계약과 사용법 (P0007 8장) |
| `internal/settings` | 설정 항목 전수 목록과 기본값 판정 (P0007 3.1, L0008 2.3·2.4) |
| `internal/params` | 모든 수치 파라미터의 단일 진실 공급원 (L0008 1장) |
| `internal/messages` | 메시지 번호와 문구 (P0007 7장) |
| `internal/control` | 제어 요청·상태 코드와 응답 판정 (P0007 1.2·1.3, L0008 2.18) |
| `internal/hooks` | 훅 이름 계약·결과 코드와 NSSM 환경 ABI (P0007 6장) |
| `internal/exitaction` | 종료 조치 되짚기 (L0008 2.5) |
| `internal/throttle` | 반복 종료 대기 계산 (L0008 2.11) |
| `internal/quote` | dump 인용 규칙 (L0008 2.7) |
| `internal/affinity` | CPU 지정 문자열과 마스크 (L0008 2.9) |
| `internal/envblock` | 환경 변수 묶음 구성·병합 (L0008 2.8) |
| `internal/cmdline` | 명령행 조립과 작업 폴더 산출 (L0008 2.6·2.10) |
| `internal/rotate` | 로그 갈아끼우기 판정·이름·실행 (L0008 2.14) |
| `internal/logrelay` | 줄머리 시각, 줄 경계 갈아끼우기, 입출력 재시도 중계 (L0008 2.15) |
| `internal/redirect` | 표준 입출력 돌리기 설정과 곧바로 잇기·중계 판정 (L0008 2.13) |
| `internal/platform` | Win32 레지스트리·SCM·계정 권한·UAC·자식 프로세스·파일 손잡이 호출을 한곳에 모은 창구 (D0006 2.5) |
| `internal/gui` | 메모리 대화상자 템플릿, 11개 설정 탭, 화면-설정 연결과 검증 |
| `internal/supervisor` | 서비스 설정 스냅샷, SCM 상태 전이, 자식·훅 기동과 감시, 재시작·로그 정책 (D0006 2.3, L0008 2.11·2.16·3장·4.4·4.7) |
| `internal/msgcat` | 메시지 목록 파일 읽기 (P0007 7장) |
| `internal/rsrc` | MESSAGETABLE·VERSIONINFO·아이콘·매니페스트와 COFF 오브젝트 쓰기 |
| `tools/mkrsrc` | 리소스 오브젝트 생성기 |
| `tools/dist.ps1` | 배포 산출물 빌드 |
| `resources/` | 원본에서 물려받은 메시지 목록과 아이콘 |
| `docs/T1-spike.md` | 화면 구현 수단·취소 가능한 대기 스파이크 결과 |
| `docs/T8-packaging.md` | 이벤트 번호 표기, 리소스 생성, 32비트, 재현 가능한 배포 |
| `docs/T9-gui.md` | 설치·편집·삭제 창과 11개 설정 탭 |

`internal/platform` 을 뺀 나머지는 운영체제에 매이지 않는 순수 판정이라 그대로 시험할 수 있다.

## 진행 단계

D0006 부록이 잡은 T/TR 9쌍 중 현재 위치.

- [x] **T1 스파이크** — 화면 수단, 취소 가능한 대기, 메시지 테이블 제약 (`docs/T1-spike.md`)
- [x] **T2 골격** — 실행 모드 판별, 명령 분배, 계약·판정 로직과 시험
- [x] **T3 저장소** — 레지스트리·SCM 읽기·쓰기, 계정 권한, UAC, 설정·설치·삭제·목록·제어 명령
- [x] **T4 기동** — 설정 스냅샷, SCM 상태 보고, 감독자, 자식 기동·감시, 재시작 정책
- [x] **T5 종료** — 단계적 종료, 생성 시각 검증 프로세스 트리 정리, `processes`
- [x] **T6 로그** — 표준 입출력 돌리기, 줄머리 시각, 시작 시·실행 중 갈아끼우기
- [x] **T7 훅** — 사건별 동기·비동기 훅 실행, 환경 전달, 제한 시간과 출력 상속
- [x] **T8 패키징** — `.syso` 생성기(메시지 테이블·버전·아이콘), 이벤트 로그 기록,
      32비트 빌드, 재현 가능한 배포 산출물 (`docs/T8-packaging.md`)
- [x] **T9 화면** — 메모리 대화상자 템플릿, 11개 설정 탭, 설치·편집·삭제 창 (`docs/T9-gui.md`)

T1부터 T9까지 계획된 이식 단계를 모두 구현했다.

## 이벤트 로그

이벤트 뷰어는 `HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Application\NSSM`
의 `EventMessageFile` 이 가리키는 실행 파일에서 문구를 찾는다. 설치할 때 그
값이 `ansm.exe` 로 바뀌므로, **기존 나씀이 남긴 과거 기록도 이 실행 파일의
메시지 표로 읽힌다.** 그래서 표는 원본과 같은 번호·같은 문구를 담는다.

기록에 실리는 번호는 `(심각도 << 30) | 번호` 다. 뷰어가 보여주는 "이벤트 ID"
는 하위 16비트라 1008 처럼 보이지만 실제 값은 1073742832 다. 자세한 사정은
`docs/T8-packaging.md` 1장에 있다.
