const { ethers } = require("hardhat");

async function deploy(name, args = []) {
  const Factory = await ethers.getContractFactory(name);
  const contract = await Factory.deploy(...args);
  await contract.waitForDeployment();
  const address = await contract.getAddress();
  console.log(`${name}:`, address);
  return contract;
}

async function wait(txPromise, label) {
  const tx = await txPromise;
  const receipt = await tx.wait();
  console.log(`${label}:`, receipt.hash);
  return receipt;
}

async function main() {
  const [deployer] = await ethers.getSigners();
  if (!deployer) {
    throw new Error("DEPLOYER_PRIVATE_KEY is required for bscTestnet");
  }

  const owner = process.env.CONTRACT_OWNER || deployer.address;
  const recipient = ethers.Wallet.createRandom().address;
  const oneUsdt = ethers.parseUnits("1", 18);
  const maxTransfer = ethers.parseUnits("10", 18);
  const dailyLimit = ethers.parseUnits("100", 18);

  console.log("network:", (await ethers.provider.getNetwork()).name);
  console.log("chainId:", (await ethers.provider.getNetwork()).chainId.toString());
  console.log("deployer:", deployer.address);
  console.log("owner:", owner);
  console.log("recipient:", recipient);

  const token = await deploy("MockBEP20", ["Mock USDT", "mUSDT", 18]);
  const vault = await deploy("SwappyTreasuryVault", [owner]);
  const registry = await deploy("SwappyDelegateRegistry", [owner]);
  const delegate = await deploy("Swappy7702PayoutDelegate");

  if (owner.toLowerCase() !== deployer.address.toLowerCase()) {
    console.log("owner is not deployer; stopping before owner-only setup");
    console.log("manual contracts:");
    console.log("MOCK_USDT=", await token.getAddress());
    console.log("TREASURY_CONTRACT=", await vault.getAddress());
    console.log("DELEGATE_REGISTRY=", await registry.getAddress());
    console.log("CUSTODY_TRUSTED_DELEGATES=", await delegate.getAddress());
    return;
  }

  await wait(token.mint(await vault.getAddress(), ethers.parseUnits("25", 18)), "mock mint to vault");
  await wait(vault.setTokenPolicy(await token.getAddress(), true, maxTransfer, dailyLimit), "vault token policy");
  await wait(vault.setOperator(deployer.address, true), "vault operator");
  await wait(vault.setRecipientAllowed(recipient, true), "vault recipient allow");

  const opId = ethers.id(`smoke:${Date.now()}`);
  await wait(vault.payout(opId, await token.getAddress(), recipient, oneUsdt), "vault payout");

  const recipientBalance = await token.balanceOf(recipient);
  if (recipientBalance !== oneUsdt) {
    throw new Error(`unexpected recipient balance: ${recipientBalance}`);
  }

  await wait(vault.pause(), "vault pause");
  await wait(vault.unpause(), "vault unpause");

  const codeHash = ethers.keccak256(await ethers.provider.getCode(await delegate.getAddress()));
  await wait(registry.trustDelegate(await delegate.getAddress(), codeHash, "Swappy7702PayoutDelegate testnet"), "registry trust delegate");

  const trusted = await registry.isTrusted(await delegate.getAddress(), codeHash);
  if (!trusted) {
    throw new Error("delegate was not trusted");
  }

  console.log("");
  console.log("SMOKE TEST OK");
  console.log("MOCK_USDT=", await token.getAddress());
  console.log("TREASURY_CONTRACT=", await vault.getAddress());
  console.log("DELEGATE_REGISTRY=", await registry.getAddress());
  console.log("CUSTODY_TRUSTED_DELEGATES=", await delegate.getAddress());
  console.log("DELEGATE_CODEHASH=", codeHash);
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
