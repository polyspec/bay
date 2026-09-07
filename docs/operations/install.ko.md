# 설치

머신당 1회 수행하는 설정이다. 이후 모든 작업은 관리자 권한 없이 실행된다.

요구 사항: Apple `container` 1.3 이상이 설치된 macOS, Go 1.27 이상. 앱은 AppKit
계층을 컴파일하는 Xcode 명령줄 도구도 필요하다.

## 명령줄만

```sh
go install github.com/polyspec/bay/cmd/bay@latest
go install github.com/polyspec/bay/cmd/bayd@latest
```

두 바이너리가 같은 디렉터리에 설치된다. `bay install`이 `bay`
옆의 `bayd` 경로로 DNS 에이전트를 등록하므로 이 배치가 필요하다.

## 명령줄과 앱

```sh
git clone https://github.com/polyspec/bay.git
cd bay
make
sudo make install
```

`make`는 `bin/bay`, `bin/bayd`, `bin/baybar.app`을
생성한다. `make install`은 바이너리 두 개를 `/usr/local/bin`에, 앱을
`/Applications`에 복사한다.

`PREFIX`와 `APPDIR`로 다른 위치에 설치할 수 있다. 자기 소유 디렉터리라면 `sudo`가
필요 없다.

```sh
make install PREFIX="$HOME/.local" APPDIR="$HOME/Applications"
```

`make uninstall`은 설치한 파일을 제거한다. 머신 설정은 되돌리지 않으므로 그건
`bay uninstall`로 한다.

## 미리 빌드한 다운로드는 제공하지 않는다

다운로드한 바이너리에는 격리 속성이 붙고, macOS는 서명되지 않은 격리 바이너리를
실행 전에 종료한다. 동작하는 다운로드를 배포하려면 Apple Developer ID 서명과
공증이 필요한데 이 프로젝트에는 없다. 위 두 경로는 머신에서 빌드하므로 격리되지
않는다.

## 머신 설정

앱을 열고 **Finish setup**을 선택하거나 다음을 실행한다.

```sh
bin/bay install
```

세 단계가 적용된다.

| 단계 | 관리자 권한 |
| --- | --- |
| 위임할 도메인마다 `/etc/resolver/<도메인>` 작성 | 필요 |
| 인증 기관을 사용자 신뢰 설정에 추가 | 불필요 |
| DNS 서버를 launchd 사용자 에이전트로 등록 | 불필요 |

명령줄은 실행된 터미널에서 `sudo`로 암호를 묻는다. 앱은 시스템 인증 창을 띄우고
번들 안의 `bay` 바이너리로 권한 단계를 실행한다.

## 확인

```sh
bin/bay doctor
```

무엇이 빠졌는지 보고하고 아무것도 바꾸지 않는다. 설정이 끝난 머신은
`nothing to do`를 출력한다.

## 제거

```sh
bay uninstall
security remove-trusted-cert -d ~/.bay/ca.crt
rm -rf ~/.bay
sudo make uninstall          # make install을 썼다면 클론 디렉터리에서
```

`uninstall`은 resolver 항목과 launchd 에이전트를 제거한다. 인증 기관은 두 번째
명령을 실행할 때까지 신뢰 설정에 남는다. 이 도구가 생성하는 모든 경로는
`docs/spec/machine-state.md`에 있다.
