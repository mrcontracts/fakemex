# FakeMex

FakeMex is a local Hyperliquid trading workspace built with Angular and Go.
Its panel-based workflow is informed by the BitMEX trade screen, while its
branding, visual system, and exchange integration are original. Market,
account, and order data use Hyperliquid's official API.

> **Networks:** FakeMex supports Hyperliquid testnet and mainnet market data,
> account data, and signed trading. It always starts on testnet. The toolbar
> **Enable Mainnet** toggle can select mainnet at runtime when its signed profile
> is complete, and every network change forces the separate Trading toggle back
> to OFF. A separate status badge always shows the active network.
>
> **Safety:** the backend binds to loopback by default. Never put a main-wallet
> private key in this repository or expose the backend to a network interface.

## Prerequisites

- Node.js 26 and npm 12
- Go 1.26
- Bash, `curl`, CA certificates, and `ss` from `iproute2`
- A C build toolchain (`build-essential` on Ubuntu) for the Go dependencies

## Install on Linux

### Ubuntu or WSL 2

Install the required system packages:

```sh
sudo apt update
sudo apt install -y build-essential ca-certificates curl git iproute2 make
```

Ubuntu's package repositories may contain older Node.js or Go releases. Install
the versions listed above using their official distributions or your preferred
version manager.

### Arch Linux

Update the system and install the runtime, compiler, and development tools from
the official repositories:

```sh
sudo pacman -Syu --needed \
  base-devel ca-certificates curl git go iproute2 nodejs npm
```

### Finish the installation

Verify the required tool versions before continuing:

```sh
node --version
npm --version
go version
```

Install the frontend and backend dependencies:

```sh
make install
```

Create the local configuration only if it does not already exist, then restrict
its permissions before adding account or API-wallet values:

```sh
test -f config/local.env || cp config/local.env.example config/local.env
chmod 600 config/local.env
```

Edit `config/local.env` as described below, then start FakeMex with
`./launch.sh`. The launcher requires ports `8080` and `4200` to be available.

## Local configuration

`config/local.env` is ignored by Git and should have mode `0600`. If it does
not exist, copy `config/local.env.example`. Each network has an independent
profile:

- `HL_TESTNET_ACCOUNT_ADDRESS` / `HL_MAINNET_ACCOUNT_ADDRESS`: the master
  account or subaccount queried on that network
- `HL_<NETWORK>_API_WALLET_ADDRESS`: the approved API-wallet/agent address for
  that network, when trading is permitted
- `HL_<NETWORK>_API_WALLET_PRIVATE_KEY`: that API wallet's private key
- `TRADING_ENABLED=true`: process-level permission for signed trading on any
  network whose credential profile is complete

The official API and WebSocket endpoints are shown in the example file and are
also the defaults. FakeMex rejects alternate upstream hosts. Existing
single-network `HL_API_URL`, `HL_WS_URL`, `HL_ACCOUNT_ADDRESS`, and API-wallet
variables are still accepted as testnet-only compatibility values; new setups
should use the network-prefixed names.

The backend derives the public address from the key at startup and refuses to
start if it does not match. It never sends credentials to Angular. Use a
dedicated API wallet—not the master wallet key.

Trading has two independent gates. `TRADING_ENABLED=true` permits signed writes
for that backend process, while the toolbar **Trading** toggle arms them at
runtime for the currently selected network. The runtime toggle starts OFF after
a backend restart and returns to OFF on every network switch. Any order, cancel,
leverage change, or position close attempted while it is off is blocked by the
backend; the UI also shows a warning popup. Selecting mainnet does not enable
trading. Re-enabling Trading on mainnet can place real orders with real funds.
Enabling mainnet is rejected unless all three `HL_MAINNET_*`
account/API-wallet credential values are present and valid. `TRADING_ENABLED`
is independent: it permits or blocks signed writes on both networks. With it
set to `false`, either configured network remains selectable in read-only mode
and its Trading toggle cannot be armed.

## Run locally

Start both services from one command (recommended):

```sh
./launch.sh
```

`launch.sh` checks required commands and required config keys, builds the backend to
`.run/fakemex`, installs frontend dependencies if needed, checks default ports, and
launches both services with strict cleanup on exit.
Running it again safely restarts this project's existing backend and frontend;
it refuses to stop unrelated processes that happen to occupy either port.

Use a different workspace-local config file when needed:

```sh
FAKEMEX_CONFIG="$PWD/config/alternate.env" ./launch.sh
```

The backend remains on `127.0.0.1:8080` because the checked-in Angular proxy
routes `/api` and WebSocket traffic there. The frontend port follows the
configured `FRONTEND_ORIGIN`.

Open <http://127.0.0.1:4200>. Angular proxies `/api` and WebSocket traffic to
the loopback Go service at `127.0.0.1:8080`.

### Windows with WSL 2

Keep the repository inside the WSL Linux filesystem so dependency installation
is fast and `chmod 600 config/local.env` works as expected. From PowerShell,
run the wrapper with the WSL distribution and Linux project path:

```powershell
.\start-wsl.ps1 -Distro Ubuntu -ProjectPath '~/FakeMex'
```

Both arguments are configurable. Omit `-Distro` to use the default WSL
distribution. The wrapper stays attached to `launch.sh`, so `Ctrl+C` performs
the launcher's normal cleanup and its exit code is returned to PowerShell.

## Verify

On Ubuntu or WSL, the first end-to-end test run also needs Playwright's Chromium
browser and its Linux system dependencies:

```sh
cd frontend
npx playwright install --with-deps chromium
cd ..
```

Playwright does not officially list Arch Linux as a supported host. On Arch,
install its Chromium build with `npx playwright install chromium`; if its system
dependencies are incompatible, run the end-to-end tests in a supported
Debian/Ubuntu container or CI environment instead. This limitation affects the
browser tests, not the application itself.

Then run the complete checks and production builds:

```sh
make test
make build
```

Backend tests mock signed exchange writes. Live exchange smoke tests are kept
separate so an ordinary test run cannot place an order accidentally.
The signing tests use public vectors from Hyperliquid's official SDK; they do
not use local credentials or contact the exchange.

The normalized local API is documented in
[`docs/api-contract.md`](docs/api-contract.md).

## Themes

The UI palette selector adapts themes distributed through Kitty's official
[themes kitten](https://sw.kovidgoyal.net/kitty/kittens/themes/) and
[kitty-themes collection](https://github.com/kovidgoyal/kitty-themes). Kitty
color roles are mapped to semantic web UI and chart colors; FakeMex does not
modify the user's Kitty configuration.
