const crypto = require("crypto");

const DEFAULT_SIGNER_URL = "http://127.0.0.1:4010";
const DEFAULT_RECIPIENT = "0x000000000000000000000000000000000000dEaD";
const DEFAULT_USDT = "0x55d398326f99059fF775485246999027B3197955";

function arg(name, fallback = "") {
  const prefix = `--${name}=`;
  const found = process.argv.find((item) => item.startsWith(prefix));
  return found ? found.slice(prefix.length) : fallback;
}

function hmacRaw(secret, ts, nonce, body) {
  return crypto.createHmac("sha256", secret).update(`${ts}.${nonce}.${body}`).digest("hex");
}

function nonce(label) {
  return `${label}-${crypto.randomBytes(12).toString("hex")}`;
}

function signedHeaders(secret, body, nonceValue = nonce("adv"), ts = Math.floor(Date.now() / 1000).toString()) {
  return {
    "content-type": "application/json",
    "x-ts": ts,
    "x-nonce": nonceValue,
    "x-signer-hmac": hmacRaw(secret, ts, nonceValue, body),
  };
}

async function request(baseURL, path, options = {}) {
  const started = Date.now();
  try {
    const res = await fetch(`${baseURL}${path}`, options);
    const text = await res.text();
    return { ok: true, status: res.status, text, ms: Date.now() - started };
  } catch (err) {
    return { ok: false, status: 0, text: err.message, ms: Date.now() - started };
  }
}

function expectStatus(name, got, allowed) {
  const pass = allowed.includes(got.status);
  return {
    name,
    pass,
    status: got.status,
    expected: allowed.join("|"),
    ms: got.ms,
    detail: got.text.slice(0, 220).replace(/\s+/g, " "),
  };
}

function printResult(result) {
  const mark = result.pass ? "PASS" : "FAIL";
  console.log(`${mark} ${result.name} status=${result.status} expected=${result.expected} ${result.ms}ms`);
  if (!result.pass || process.env.VERBOSE === "1") {
    console.log(`  ${result.detail}`);
  }
}

async function main() {
  const baseURL = (arg("url", process.env.SIGNER_URL) || DEFAULT_SIGNER_URL).replace(/\/+$/, "");
  const secret = arg("secret", process.env.SIGNER_HMAC_SECRET || process.env.HMAC_SECRET || "");
  const token = arg("token", process.env.BSC_USDT_CONTRACT || DEFAULT_USDT);
  const recipient = arg("to", DEFAULT_RECIPIENT);
  const network = arg("network", "BSC");

  const results = [];
  results.push(expectStatus("healthz is public", await request(baseURL, "/healthz"), [200]));
  results.push(expectStatus("readyz is public", await request(baseURL, "/readyz"), [200, 503]));

  const bodyObject = {
    to: recipient,
    amount: "0",
    tokenContract: token,
    network,
    idempotencyKey: `adv-${Date.now()}-${crypto.randomBytes(4).toString("hex")}`,
  };
  const body = JSON.stringify(bodyObject);

  results.push(
    expectStatus(
      "protected transfer rejects missing HMAC",
      await request(baseURL, "/hd/transfer", { method: "POST", body, headers: { "content-type": "application/json" } }),
      [401],
    ),
  );

  results.push(
    expectStatus(
      "protected transfer rejects bad HMAC",
      await request(baseURL, "/hd/transfer", {
        method: "POST",
        body,
        headers: { "content-type": "application/json", "x-ts": `${Math.floor(Date.now() / 1000)}`, "x-nonce": nonce("bad"), "x-signer-hmac": "00" },
      }),
      [401],
    ),
  );

  if (secret) {
    const oldTs = `${Math.floor(Date.now() / 1000) - 3600}`;
    results.push(
      expectStatus(
        "protected transfer rejects expired signed request",
        await request(baseURL, "/hd/transfer", {
          method: "POST",
          body,
          headers: signedHeaders(secret, body, nonce("expired"), oldTs),
        }),
        [401],
      ),
    );

    const signedOriginal = signedHeaders(secret, body, nonce("tamper"));
    const tamperedBody = JSON.stringify({ ...bodyObject, to: "0x1111111111111111111111111111111111111111" });
    results.push(
      expectStatus(
        "protected transfer rejects tampered payload",
        await request(baseURL, "/hd/transfer", { method: "POST", body: tamperedBody, headers: signedOriginal }),
        [401],
      ),
    );

    const replayNonce = nonce("replay");
    const replayHeaders = signedHeaders(secret, body, replayNonce);
    results.push(
      expectStatus(
        "valid HMAC reaches policy and rejects zero amount without sending",
        await request(baseURL, "/hd/transfer", { method: "POST", body, headers: replayHeaders }),
        [400, 409, 502],
      ),
    );
    results.push(
      expectStatus(
        "same nonce replay is rejected",
        await request(baseURL, "/hd/transfer", { method: "POST", body, headers: replayHeaders }),
        [401],
      ),
    );
  } else {
    console.log("SKIP signed adversarial cases: set SIGNER_HMAC_SECRET or pass --secret=...");
  }

  for (const result of results) printResult(result);
  const failed = results.filter((result) => !result.pass);
  console.log("");
  console.log(`Signer adversarial smoke finished: ${results.length - failed.length}/${results.length} passed`);
  if (failed.length) process.exitCode = 1;
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
