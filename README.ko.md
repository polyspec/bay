# bay

`bay`은 Apple `container` 위에서 Compose 프로젝트를 실행하고, 각
서비스를 로컬 도메인 이름으로 HTTPS 제공한다. 호스트 포트를 게시하지 않고
`/etc/hosts`에 항목을 추가하지 않는다.

프로젝트는 Compose 파일이다. 추가 설정이 없는 서비스는 `<서비스>.test`로
제공된다.

## 시작

```sh
go install github.com/polyspec/bay/cmd/bay@latest
go install github.com/polyspec/bay/cmd/bayd@latest
bay install         # 머신당 1회. 암호를 묻는다
cd ~/work/your-project
bay up
```

메뉴바 앱까지 쓰려면 저장소를 클론해 `make`와 `sudo make install`을 실행한다.
[설치](docs/operations/install.ko.md) 참고.

## 사용법은 명령에 있다

```sh
bay brief           # 전체 사용 계약. 프로그램용은 --json
bay schema          # Compose 파일 계약. 프로그램용은 --json
bay help up         # 명령 하나가 바꾸는 것
bay status --json   # 현재 머신 상태
```

바이너리를 설치한 프로젝트는 이 저장소를 받지 않으므로 명령이 사용법을 담는다.

## 문서

| 문서 | 내용 |
| --- | --- |
| [설치](docs/operations/install.ko.md) | 1회 머신 설정과 제거 방법 |
| [사용](docs/operations/using.ko.md) | 프로젝트 추가, 도메인, 포트, 일상 명령 |
| [문제 해결](docs/operations/troubleshooting.ko.md) | 증상, 원인, 조치 |
| [구조](docs/spec/architecture.md) | 구성 요소, 요청 경로, 라우팅 상태 |
| [명령 계약](docs/spec/cli.md) | 명령, 효과, 규칙, 진단 |
| [Compose 계약](docs/spec/compose-schema.md) | 키, 라벨, 포트 결정 |
| [머신 상태](docs/spec/machine-state.md) | 저장소 밖에 생성되는 모든 경로 |
| [기능](docs/features.ko.md) | 구현 상태와 검증 |
| [변경 기록](CHANGELOG.ko.md) | 동작 변경과 검증 |
| [문서 계획](docs/documentation-plan.ko.md) | 어떤 정보를 어디에 적는가 |
| [개발](AGENTS.md) | 빌드, 테스트, 필수 검사 |

명세 문서는 코드에서 생성되는 표를 담고 있어 영어만 유지한다.

## 요구 사항

Apple `container` 1.3 이상이 설치된 macOS, Go 1.27 이상. 앱은 Xcode 명령줄
도구도 필요하다.
