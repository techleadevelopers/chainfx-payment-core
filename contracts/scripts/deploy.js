const { ethers } = require("hardhat");

async function main() {
  const [deployer] = await ethers.getSigners();
  const owner = process.env.CONTRACT_OWNER || deployer.address;
  const usdt = process.env.BSC_USDT_CONTRACT || "";

  console.log("deployer:", deployer.address);
  console.log("owner:", owner);

  const Vault = await ethers.getContractFactory("SwappyTreasuryVault");
  const vault = await Vault.deploy(owner);
  await vault.waitForDeployment();
  const vaultAddress = await vault.getAddress();
  console.log("SwappyTreasuryVault:", vaultAddress);

  const Registry = await ethers.getContractFactory("SwappyDelegateRegistry");
  const registry = await Registry.deploy(owner);
  await registry.waitForDeployment();
  const registryAddress = await registry.getAddress();
  console.log("SwappyDelegateRegistry:", registryAddress);

  const Delegate = await ethers.getContractFactory("Swappy7702PayoutDelegate");
  const delegate = await Delegate.deploy();
  await delegate.waitForDeployment();
  const delegateAddress = await delegate.getAddress();
  console.log("Swappy7702PayoutDelegate:", delegateAddress);

  if (owner === deployer.address && usdt) {
    const maxTransfer = ethers.parseUnits(process.env.TREASURY_MAX_TRANSFER_USDT || "100", 18);
    const dailyLimit = ethers.parseUnits(process.env.TREASURY_DAILY_LIMIT_USDT || "1000", 18);
    const tx = await vault.setTokenPolicy(usdt, true, maxTransfer, dailyLimit);
    await tx.wait();
    console.log("USDT policy configured:", usdt);
  } else {
    console.log("manual next step: configure token policy from owner wallet");
  }

  console.log("");
  console.log("env suggestions:");
  console.log(`CUSTODY_TRUSTED_DELEGATES=${delegateAddress}`);
  console.log(`TREASURY_CONTRACT=${vaultAddress}`);
  console.log(`DELEGATE_REGISTRY=${registryAddress}`);
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
