# FakeMex

FakeMex is a local, testnet-only trading workspace built with Angular and Go.
Its panel-based workflow is informed by the BitMEX trade screen, while its
branding, visual system, and exchange integration are original. Market and
account data come from Hyperliquid testnet.

> **Safety:** the backend is intentionally locked to Hyperliquid testnet and
> binds to loopback by default. Never put a main-wallet private key in this
> repository or expose the backend to a network interface.

## Prerequisites

- Node.js 26 and npm 12
- Go 1.26

## Local configuration

`config/local.env` is ignored by Git and should have mode `0600`. If it does
not exist, copy `config/local.env.example` and fill in:

- `HL_ACCOUNT_ADDRESS`: the master account or subaccount queried for state
- `TRADING_ENABLED=true`: permits signed testnet trading in the backend
- `HL_API_WALLET_ADDRESS`: the approved API-wallet/agent public address, when trading is permitted
- `HL_API_WALLET_PRIVATE_KEY`: that API wallet's private key, when trading is permitted

The backend derives the public address from the key at startup and refuses to
start if it does not match. It never sends credentials to Angular. Use a
dedicated testnet API wallet—not the master wallet key.

Trading has two independent gates. `TRADING_ENABLED=true` permits signed writes
for that backend process, while the toolbar **Trading** toggle arms them at
runtime. The runtime toggle always starts off after a backend restart. Any order,
cancel, leverage change, or position close attempted while it is off is blocked
by the backend; the UI also shows a warning popup.

## Run locally

Start both services from one command (recommended):

```sh
./launch.sh
```

`launch.sh` checks required commands and required config keys, builds the backend to
`.run/fakemex`, installs frontend dependencies if needed, checks default ports, and
launches both services with strict cleanup on exit.

Use a different workspace-local config file when needed:

```sh
FAKEMEX_CONFIG="$PWD/config/alternate.env" ./launch.sh
```

The backend remains on `127.0.0.1:8080` because the checked-in Angular proxy
routes `/api` and WebSocket traffic there. The frontend port follows the
configured `FRONTEND_ORIGIN`.

Open <http://127.0.0.1:4200>. Angular proxies `/api` and WebSocket traffic to
the loopback Go service at `127.0.0.1:8080`.

## Verify

```sh
make test
make build
```

Backend tests mock signed exchange writes. Live testnet smoke tests are kept
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
