# ChainFX Python SDK

Minimal Python SDK for the ChainFX Digital FX Payments API.

```python
import os
from chainfx import ChainFX

chainfx = ChainFX(
    api_key=os.environ["CHAINFX_API_KEY"],
    base_url=os.getenv("CHAINFX_API_BASE_URL", "https://sandbox-api.chainfx.com"),
)

quote = chainfx.quote(side="buy", fiat="BRL", asset="USDT", amount=500)

order = chainfx.buy(
    fiat="BRL",
    asset="USDT",
    amount=500,
    wallet="0x000000000000000000000000000000000000dEaD",
    customer={
        "name": "Maria Silva",
        "email": "maria@example.com",
        "cpf": "12345678909",
        "phone": "11999999999",
        "birthDate": "1990-05-20",
        "address": {
            "line1": "Av Paulista",
            "number": "1000",
            "city": "Sao Paulo",
            "state": "SP",
            "postalCode": "01310100",
            "country": "BR",
        },
    },
)
```

No external dependency is required; it uses Python standard library HTTP clients.

## Current backend coverage

This SDK is intentionally minimal today: quote, buy and sell primitives. The backend now also exposes production endpoints that can be called directly until the SDK grows wrappers:

| Area | Endpoints |
| --- | --- |
| Efi credit-card buy | `POST /api/buy` with `paymentMethod=credit_card`, `paymentToken`, `customer`, `billingAddress` |
| MCP Capability Network | `POST /mcp/initialize`, `POST /mcp/tools/list`, `POST /mcp/tools/call` |
| Agent Pay | `POST /agent/v1/pay`, `GET /agent/v1/pay/{id}` |
| Gas Station | `GET /v1/gas/status`, `GET /v1/gas/quote`, `POST /v1/gas/relay`, `GET /v1/gas/relay/{id}` |

Agents and developer backends should use Bearer API keys issued by the Developer Console.
