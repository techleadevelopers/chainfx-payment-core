const { expect } = require("chai");
const { ethers } = require("hardhat");

describe("SwappyTreasuryVault", function () {
  async function deployFixture() {
    const [owner, operator, guardian, customer, attacker] = await ethers.getSigners();

    const Token = await ethers.getContractFactory("MockBEP20");
    const token = await Token.deploy("Mock USDT", "USDT", 18);
    await token.waitForDeployment();

    const Vault = await ethers.getContractFactory("SwappyTreasuryVault");
    const vault = await Vault.deploy(owner.address);
    await vault.waitForDeployment();

    const tokenAddress = await token.getAddress();
    const vaultAddress = await vault.getAddress();
    await token.mint(vaultAddress, ethers.parseUnits("10000", 18));
    await vault.setOperator(operator.address, true);
    await vault.setGuardian(guardian.address, true);
    await vault.setRecipientAllowed(customer.address, true);
    await vault.setTokenPolicy(tokenAddress, true, ethers.parseUnits("100", 18), ethers.parseUnits("500", 18));

    return { owner, operator, guardian, customer, attacker, token, tokenAddress, vault };
  }

  it("allows operator payout to allowed recipient", async function () {
    const { operator, customer, token, tokenAddress, vault } = await deployFixture();
    const opId = ethers.id("buy-1");

    await expect(vault.connect(operator).payout(opId, tokenAddress, customer.address, ethers.parseUnits("20", 18)))
      .to.emit(vault, "Payout");

    expect(await token.balanceOf(customer.address)).to.equal(ethers.parseUnits("20", 18));
  });

  it("blocks duplicate operation id", async function () {
    const { operator, customer, tokenAddress, vault } = await deployFixture();
    const opId = ethers.id("buy-duplicate");

    await vault.connect(operator).payout(opId, tokenAddress, customer.address, ethers.parseUnits("1", 18));
    await expect(
      vault.connect(operator).payout(opId, tokenAddress, customer.address, ethers.parseUnits("1", 18))
    ).to.be.revertedWithCustomError(vault, "OperationAlreadyExecuted");
  });

  it("blocks non-allowed recipient", async function () {
    const { operator, attacker, tokenAddress, vault } = await deployFixture();

    await expect(
      vault.connect(operator).payout(ethers.id("bad-recipient"), tokenAddress, attacker.address, ethers.parseUnits("1", 18))
    ).to.be.revertedWithCustomError(vault, "RecipientNotAllowed");
  });

  it("guardian can pause payouts", async function () {
    const { operator, guardian, customer, tokenAddress, vault } = await deployFixture();

    await vault.connect(guardian).pause();
    await expect(
      vault.connect(operator).payout(ethers.id("paused"), tokenAddress, customer.address, ethers.parseUnits("1", 18))
    ).to.be.revertedWithCustomError(vault, "PausedError");
  });

  it("enforces daily limit", async function () {
    const { operator, customer, tokenAddress, vault } = await deployFixture();

    await vault.connect(operator).payout(ethers.id("daily-1"), tokenAddress, customer.address, ethers.parseUnits("100", 18));
    await vault.connect(operator).payout(ethers.id("daily-2"), tokenAddress, customer.address, ethers.parseUnits("100", 18));
    await vault.connect(operator).payout(ethers.id("daily-3"), tokenAddress, customer.address, ethers.parseUnits("100", 18));
    await vault.connect(operator).payout(ethers.id("daily-4"), tokenAddress, customer.address, ethers.parseUnits("100", 18));
    await vault.connect(operator).payout(ethers.id("daily-5"), tokenAddress, customer.address, ethers.parseUnits("100", 18));

    await expect(
      vault.connect(operator).payout(ethers.id("daily-6"), tokenAddress, customer.address, ethers.parseUnits("1", 18))
    ).to.be.revertedWithCustomError(vault, "DailyLimitExceeded");
  });
});
