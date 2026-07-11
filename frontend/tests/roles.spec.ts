import type { Browser, BrowserContext, Locator, Page } from "@playwright/test";
import { expect, test } from "@playwright/test";

const password = "DevPassword123!";
const seededTransferCode = "TRF-KNP-MNS-0002";
const seededDispatchTransfer = "TRF-MNS-KNP-0003 / requested";

type SessionHandle = {
  context: BrowserContext;
  page: Page;
};

async function signIn(page: Page, email: string, expectedPath: string) {
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign In" }).click();
  await page.waitForURL(new RegExp(`${expectedPath}$`));
}

async function openSession(browser: Browser, email: string, expectedPath: string): Promise<SessionHandle> {
  const context = await browser.newContext();
  const page = await context.newPage();
  await signIn(page, email, expectedPath);
  return { context, page };
}

async function chooseOption(scope: Page | Locator, page: Page, label: string, optionText: string) {
  await scope.getByLabel(label).click();
  await page.getByRole("option", { name: optionText, exact: true }).click();
}

function card(page: Page, title: string) {
  return page.locator("section").filter({
    has: page.getByRole("heading", { name: title, exact: true })
  }).first();
}

test.describe("role and workflow coverage", () => {
  test.describe.configure({ mode: "serial" });
  test.skip(!process.env.E2E_RUN, "Enable E2E_RUN=1 when services are up");

  test("three roles can sign in simultaneously with role-based navigation", async ({ browser }) => {
    const sessions = await Promise.all([
      openSession(browser, "superadmin@erp.local", "/dashboard"),
      openSession(browser, "branchadmin@erp.local", "/branch-dashboard"),
      openSession(browser, "pos@erp.local", "/sales")
    ]);

    const [superAdmin, branchAdmin, pos] = sessions.map((session) => session.page);

    await expect(superAdmin.getByRole("link", { name: /Dashboard/ })).toBeVisible();
    await expect(superAdmin.getByRole("link", { name: /Global Reports/ })).toBeVisible();
    await expect(superAdmin.getByRole("link", { name: /Settings/ })).toBeVisible();
    await expect(superAdmin.getByRole("link", { name: /Audit/ })).toHaveCount(0);
    await expect(branchAdmin.getByRole("link", { name: /Sales & Invoices/ })).toBeVisible();
    await expect(branchAdmin.getByRole("link", { name: /Local Finance/ })).toBeVisible();
    await expect(branchAdmin.getByRole("link", { name: /Branch Inventory/ })).toBeVisible();
    await expect(branchAdmin.getByRole("link", { name: /Transfers/ })).toHaveCount(0);
    await expect(branchAdmin.getByRole("link", { name: /Settings/ })).toHaveCount(0);
    await expect(pos.getByRole("link", { name: /POS Screen/ })).toBeVisible();
    await expect(pos.getByRole("link", { name: /Inventory Check/ })).toBeVisible();
    await expect(pos.getByRole("link", { name: /Goods Transfer Receipt/ })).toBeVisible();
    await expect(pos.getByRole("link", { name: /Daily Sales Summary/ })).toBeVisible();
    await expect(pos.getByRole("link", { name: /Settings/ })).toHaveCount(0);

    await Promise.all(sessions.map((session) => session.context.close()));
  });

  test("branch POS can preview and create a retail sale", async ({ browser }) => {
    const session = await openSession(browser, "pos@erp.local", "/sales");
    const composer = card(session.page, "Create POS Invoice");

    await composer.getByLabel("Customer Name").fill("ลูกค้าทดสอบ E2E");
    await chooseOption(composer, session.page, "Product 1", "หน้ากากอนามัย");
    await composer.getByRole("button", { name: "Preview" }).click();
    await expect(composer.getByText(/95\.23/)).toBeVisible();
    await composer.getByRole("button", { name: "Create Invoice" }).click();
    await expect(session.page.getByText("invoice created")).toBeVisible();

    await session.context.close();
  });

  test("branch POS can preview and create a government-mode sale", async ({ browser }) => {
    const session = await openSession(browser, "pos@erp.local", "/sales");
    const composer = card(session.page, "Create POS Invoice");

    await composer.getByLabel("Customer Name").fill("องค์การบริหารส่วนตำบลทดสอบ");
    await composer.getByLabel("Tax ID").fill("0107567000001");
    await composer.getByLabel("Government mode").check();
    await chooseOption(composer, session.page, "Product 1", "เตียงผู้ป่วยปรับระดับ");
    await composer.getByLabel("Alias 1").click();
    await expect(session.page.getByRole("option", { name: "ผ้าอ้อมผู้ป่วย", exact: true })).toBeVisible();
    await session.page.getByRole("option", { name: "ผ้าอ้อมผู้ป่วย", exact: true }).click();
    await composer.getByRole("button", { name: "Preview" }).click();
    await expect(composer.getByText(/5,564\.00/)).toBeVisible();
    await composer.getByRole("button", { name: "Create Invoice" }).click();
    await expect(session.page.getByText("invoice created")).toBeVisible();

    await session.context.close();
  });

  test("branch POS can receive a transfer by manual code", async ({ browser }) => {
    const session = await openSession(browser, "pos@erp.local", "/sales");
    await session.page.goto("/transfer-receipts");

    const receiveCard = card(session.page, "Goods Transfer Receipt");
    await receiveCard.getByLabel("Transfer Code").fill(seededTransferCode);
    await receiveCard.getByRole("button", { name: "Receive by Code" }).click();
    await expect(receiveCard.getByText("Transfer received")).toBeVisible();

    await session.context.close();
  });

  test("branch admin can reconcile a seeded outstanding invoice with a check", async ({ browser }) => {
    const session = await openSession(browser, "branchadmin@erp.local", "/branch-dashboard");
    await session.page.goto("/local-finance");

    const financeCard = card(session.page, "Check Reconciliation");
    await financeCard.getByLabel("Check Number").fill(`CHK-E2E-${Date.now()}`);
    await financeCard.getByLabel("Check Amount").fill("5564");
    await financeCard.locator('input[type="checkbox"]').first().check();
    await financeCard.getByRole("button", { name: "Preview Apply" }).click();
    await expect(financeCard.getByText(/5,564\.00/).first()).toBeVisible();
    await financeCard.getByRole("button", { name: "Save Check" }).click();
    await expect(financeCard.getByText("Check saved")).toBeVisible();

    await session.context.close();
  });

  test("branch admin can dispatch a seeded transfer queue item", async ({ browser }) => {
    const session = await openSession(browser, "branchadmin@erp.local", "/branch-dashboard");
    await session.page.goto("/branch-inventory?tab=transfers");

    const dispatchCard = card(session.page, "Dispatch Queue");
    await chooseOption(dispatchCard, session.page, "Transfer ID", seededDispatchTransfer);
    await dispatchCard.getByLabel("Dispatch Pickup Name").fill("Somchai Dispatcher");
    await dispatchCard.getByLabel("Dispatch Courier Name").fill("ERP Van 1");
    await dispatchCard.getByRole("button", { name: "Dispatch" }).click();
    await expect(dispatchCard.getByText("Transfer dispatched")).toBeVisible();

    await session.context.close();
  });

  test("branch admin can open invoice reprint view", async ({ browser }) => {
    const session = await openSession(browser, "branchadmin@erp.local", "/branch-dashboard");
    await session.page.goto("/sales-invoices");
    await expect(session.page.getByText("Collect Full Payment")).toHaveCount(0);
    await expect(card(session.page, "Invoice History").getByRole("link", { name: "Print / Reprint" }).first()).toBeVisible();
    await session.context.close();
  });

  test("branch POS daily sales page is available", async ({ browser }) => {
    const session = await openSession(browser, "pos@erp.local", "/sales");
    await session.page.goto("/daily-sales");
    await expect(session.page.getByRole("heading", { name: /Daily Sales Summary/i })).toBeVisible();
    await expect(card(session.page, "Your Recent Invoices")).toBeVisible();
    await session.context.close();
  });
});
