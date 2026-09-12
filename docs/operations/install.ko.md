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

## 기계 하나에 설정 하나

`-state`는 인증기관·인증서·프로젝트 등록부가 있는 디렉터리를 고른다. 리졸버 항목,
DNS 에이전트, 프록시는 기계의 것이고 각각 하나씩이므로 상태 디렉터리 하나가
그것들의 주인이다. `bay doctor`가 주인을 알려준다. 다른 상태
디렉터리에서 돌린 명령은 기계를 읽지만 설정을 바꾸지 않는다. 멈추고 누구의
것인지 말한다. 넘겨받으려면 `bay -state <디렉터리> install`을 명시적으로
실행한다.

## 무엇이 암호를 묻는가

`/etc/resolver`에 쓸 때만 관리자 권한이 필요하다. 그 밖에는 없다. 그것도 명령마다가
아니라 도메인마다 한 번이다. 이미 위임된 도메인 아래에서 제공되는 프로젝트는 묻는
것 없이 실행된다. 무언가를 묻게 될 명령을 돌리기 전에 `bay doctor`로 남은
것을 확인한다.

스크립트나 터미널이 없는 프로그램에서는 새 도메인을 위임할 때 암호를 물을 곳이
없다. `bay install`을 터미널에서 한 번 실행하면 그 뒤의 명령은 아무것도
필요하지 않다.
