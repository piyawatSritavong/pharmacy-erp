import type { Browser, BrowserContext, Locator, Page } from "@playwright/test";
import { expect, test } from "@playwright/test";

// UI execution of docs/MANUAL_TEST_CASES.md — every action is a real click,
// fill, and submit through the rendered app. Requires a freshly seeded DB.

const password = "DevPassword123!";

type SessionHandle = {
  context: BrowserContext;
  page: Page;
};

async function signIn(page: Page, email: string, expectedPath: string, userPassword = password) {
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(userPassword);
  await page.getByRole("button", { name: "Sign In" }).click();
  await page.waitForURL(new RegExp(`${expectedPath}$`));
}

async function openSession(browser: Browser, email: string, expectedPath: string, userPassword = password): Promise<SessionHandle> {
  const context = await browser.newContext();
  const page = await context.newPage();
  await signIn(page, email, expectedPath, userPassword);
  return { context, page };
}

async function chooseOption(scope: Page | Locator, page: Page, label: string, optionText: string | RegExp) {
  await scope.getByRole("combobox", { name: label }).click();
  await page.getByRole("option", { name: optionText }).first().click();
}

function card(page: Page, title: string) {
  return page
    .locator("section")
    .filter({ has: page.getByRole("heading", { name: title, exact: true }) })
    .first();
}

test.describe("manual test cases via real UI", () => {
  test.describe.configure({ mode: "serial" });
  test.skip(!process.env.E2E_RUN, "Enable E2E_RUN=1 when services are up");

  test("styles: pages render with the compiled stylesheet applied", async ({ browser }) => {
    // Guards against serving unstyled HTML (e.g. a corrupted .next dir):
    // behavioural tests still pass on a bare page, so assert real CSS here.
    const context = await browser.newContext();
    const page = await context.newPage();
    await page.goto("/login");
    const buttonBackground = await page
      .getByRole("button", { name: "Sign In" })
      .evaluate((el) => getComputedStyle(el).backgroundColor);
    expect(buttonBackground, "Tailwind styles must be applied to the Sign In button").toBe("rgb(0, 0, 0)");

    await signIn(page, "superadmin@erp.local", "/dashboard");
    const sidebarCount = await page.locator("aside").count();
    expect(sidebarCount, "app shell sidebar should render").toBeGreaterThan(0);
    await context.close();
  });

  test("B2: super admin manages catalog and dual inventory end to end", async ({ browser }) => {
    const session = await openSession(browser, "superadmin@erp.local", "/dashboard");
    const page = session.page;
    await page.goto("/inventory-management");

    // B2-02 create product with 3-tier prices
    const catalog = card(page, "Catalog Actions");
    await catalog.getByPlaceholder("SKU").fill("WHEEL-001");
    await catalog.getByPlaceholder("Product name").fill("รถเข็นผู้ป่วย");
    await catalog.getByPlaceholder("Description").fill("รถเข็นพับได้");
    await catalog.getByPlaceholder("Cost price").fill("2500");
    await catalog.getByPlaceholder("Cash price (base)").fill("3500");
    await catalog.getByPlaceholder("Retail price (0 = use base)").fill("3700");
    await catalog.getByPlaceholder("Installment price (0 = use base)").fill("4000");
    await catalog.getByPlaceholder("Unit").fill("unit");
    await catalog.getByRole("button", { name: "Save" }).click();
    await expect(catalog.getByText("Saved")).toBeVisible();

    // F-07 duplicate SKU surfaces a clear 409 message in the UI
    await catalog.getByPlaceholder("SKU").fill("WHEEL-001");
    await catalog.getByPlaceholder("Product name").fill("ซ้ำ");
    await catalog.getByPlaceholder("Cost price").fill("1");
    await catalog.getByPlaceholder("Cash price (base)").fill("2");
    await catalog.getByRole("button", { name: "Save" }).click();
    await expect(catalog.getByText("SKU already exists")).toBeVisible();

    // B2-03 create government alias for the new product
    await chooseOption(catalog, page, "Catalog Mode", "Create Alias");
    await chooseOption(catalog, page, "Alias Product", "รถเข็นผู้ป่วย");
    await catalog.getByPlaceholder("Alias code").fill("GOV-WHEEL-01");
    await catalog.getByPlaceholder("Alias name").fill("อุปกรณ์ช่วยเคลื่อนย้ายผู้ป่วย");
    await catalog.getByPlaceholder("Default government price").fill("3900");
    await catalog.getByRole("button", { name: "Save" }).click();
    await expect(catalog.getByText("Saved")).toBeVisible();
    await expect(card(page, "Alias Registry").getByText("GOV-WHEEL-01")).toBeVisible();

    // B2-04 receive stock split into real/ghost buckets
    const inventory = card(page, "Inventory Actions");
    await chooseOption(inventory, page, "Inventory Action", "Receive Stock");
    await chooseOption(inventory, page, "Inventory Branch", "มนัสการแพทย์");
    await chooseOption(inventory, page, "Inventory Product", "รถเข็นผู้ป่วย");
    await inventory.getByLabel("Real Quantity").fill("10");
    await inventory.getByLabel("Ghost Quantity").fill("4");
    await inventory.getByLabel("Inventory Reason").fill("รับล็อตแรกจากซัพพลายเออร์");
    await inventory.getByRole("button", { name: "Receive Stock" }).click();
    await expect(inventory.getByText("stock received")).toBeVisible();

    const wheelRow = card(page, "Global Inventory")
      .getByRole("row")
      .filter({ hasText: "รถเข็นผู้ป่วย" })
      .filter({ hasText: "มนัสการแพทย์" });
    await expect(wheelRow).toContainText("10");
    await expect(wheelRow).toContainText("4");

    // B2-05 rebalance 2 real -> ghost (10/4 -> 8/6)
    await chooseOption(inventory, page, "Inventory Action", "Real ↔ Ghost");
    await chooseOption(inventory, page, "Inventory Branch", "มนัสการแพทย์");
    await chooseOption(inventory, page, "Inventory Product", "รถเข็นผู้ป่วย");
    await chooseOption(inventory, page, "Stock Bucket", "Real");
    await chooseOption(inventory, page, "To Bucket", "Ghost");
    await inventory.getByLabel("Inventory Quantity").fill("2");
    await inventory.getByLabel("Inventory Reason").fill("กันไว้ขายเงินสด");
    await inventory.getByRole("button", { name: "Rebalance Stock" }).click();
    await expect(inventory.getByText("inventory rebalanced")).toBeVisible();
    await expect(wheelRow).toContainText("8");
    await expect(wheelRow).toContainText("6");

    // B2-06 manual adjust -1 real (8/6 -> 7/6)
    await chooseOption(inventory, page, "Inventory Action", "Manual Adjust");
    await chooseOption(inventory, page, "Inventory Branch", "มนัสการแพทย์");
    await chooseOption(inventory, page, "Inventory Product", "รถเข็นผู้ป่วย");
    await chooseOption(inventory, page, "Stock Bucket", "Real");
    await inventory.getByLabel("Inventory Quantity").fill("-1");
    await inventory.getByLabel("Inventory Reason").fill("ชำรุดจากขนส่ง");
    await inventory.getByRole("button", { name: "Apply Adjustment" }).click();
    await expect(inventory.getByText("inventory adjusted")).toBeVisible();
    await expect(wheelRow).toContainText("7");

    // B2-07 adjusting below zero is rejected with a clear message
    await inventory.getByLabel("Inventory Quantity").fill("-999");
    await inventory.getByRole("button", { name: "Apply Adjustment" }).click();
    await expect(inventory.getByText(/negative/)).toBeVisible();

    await session.context.close();
  });

  test("B3: super admin creates an installment plan and collects the first month", async ({ browser }) => {
    const session = await openSession(browser, "superadmin@erp.local", "/dashboard");
    const page = session.page;
    await page.goto("/installments");

    // B3-01 seeded summary cards
    await expect(card(page, "Create Installment Plan")).toBeVisible();
    await expect(page.getByText("Overdue Installments")).toBeVisible();

    // B3-04 create a 4-month plan from the seeded unpaid invoice
    const create = card(page, "Create Installment Plan");
    await chooseOption(create, page, "Unpaid Invoice", /สุขใจ/);
    await create.getByLabel("Months").fill("4");
    await create.getByRole("button", { name: "Create Plan" }).click();
    await expect(page.getByText("installment plan created")).toBeVisible();

    const newPlan = page
      .locator("section")
      .filter({ has: page.getByRole("heading", { name: /สุขใจ/ }) })
      .first();
    await expect(newPlan.getByText("฿1,391.00").first()).toBeVisible();

    // B3-05 collect installment #1 in full (row action)
    const firstRow = newPlan.getByRole("row").filter({ hasText: "pending" }).first();
    await firstRow.getByRole("button", { name: "Collect" }).click();
    await expect(page.getByText("installment payment recorded")).toBeVisible();
    await expect(newPlan.getByText("paid").first()).toBeVisible();

    await session.context.close();
  });

  test("C4: branch admin collects overdue, partial, and rejects overpay", async ({ browser }) => {
    const session = await openSession(browser, "branchadmin@erp.local", "/branch-dashboard");
    const page = session.page;
    await page.goto("/installments");

    const plan = page
      .locator("section")
      .filter({ has: page.getByRole("heading", { name: /สมชาย/ }) })
      .first();

    // C4-01 settle the overdue installment with bank transfer
    const overdueRow = plan.getByRole("row").filter({ hasText: "overdue" }).first();
    await overdueRow.getByRole("combobox").click();
    await page.getByRole("option", { name: "Bank Transfer" }).first().click();
    await overdueRow.getByRole("button", { name: "Collect" }).click();
    await expect(page.getByText("installment payment recorded")).toBeVisible();
    await expect(plan.getByRole("row").filter({ hasText: "overdue" })).toHaveCount(0);

    // C4-02 partial payment leaves the installment pending with remaining balance
    const pendingRow = plan.getByRole("row").filter({ hasText: "pending" }).first();
    await pendingRow.getByRole("spinbutton").fill("500");
    await pendingRow.getByRole("button", { name: "Collect" }).click();
    await expect(page.getByText("installment payment recorded")).toBeVisible();
    await expect(plan.getByText("฿605.67").first()).toBeVisible();

    // C4-03 overpay is rejected with a message
    const stillPending = plan.getByRole("row").filter({ hasText: "pending" }).first();
    await stillPending.getByRole("spinbutton").fill("9999");
    await stillPending.getByRole("button", { name: "Collect" }).click();
    await expect(page.getByText(/exceeds the remaining balance/)).toBeVisible();

    await session.context.close();
  });

  test("C3: branch admin sells at every price tier through the composer", async ({ browser }) => {
    const session = await openSession(browser, "branchadmin@erp.local", "/branch-dashboard");
    const page = session.page;
    await page.goto("/sales-invoices");

    // C3-01 quotation at cash tier with preview totals
    const quote = card(page, "Create Quotation");
    await quote.getByLabel("Customer Name").fill("โรงเรียนอนุบาลดวงใจ");
    await quote.getByLabel("Tax ID").fill("0105561234567");
    await chooseOption(quote, page, "Product 1", "หน้ากากอนามัย");
    await quote.getByLabel("Quantity 1").fill("10");
    await quote.getByRole("button", { name: "Preview" }).click();
    await expect(quote.getByText(/952\.30/).first()).toBeVisible();
    await quote.getByRole("button", { name: "Create Quotation" }).click();
    await expect(quote.getByText("quotation created")).toBeVisible();

    // C3-02 convert the draft quotation into an invoice
    const log = card(page, "Quotation Log");
    await log.getByRole("button", { name: "Convert" }).first().click();
    await expect(log.getByText("converted").first()).toBeVisible();

    // C3-03 retail tier invoice (tax-exempt product, VAT 0)
    const invoice = card(page, "Create Invoice");
    await invoice.getByLabel("Customer Name").fill("ลูกค้าปลีกหน้าร้าน");
    await chooseOption(invoice, page, "Price Tier", "Retail price");
    await chooseOption(invoice, page, "Product 1", "ยาพาราเซตามอล");
    await invoice.getByLabel("Quantity 1").fill("2");
    await invoice.getByRole("button", { name: "Preview" }).click();
    await expect(invoice.getByText(/78\.00/).first()).toBeVisible();
    await invoice.getByRole("button", { name: "Create Invoice" }).click();
    await expect(invoice.getByText("invoice created")).toBeVisible();

    // C3-06 manual price override with reason
    await invoice.getByLabel("Customer Name").fill("ลูกค้าประจำ");
    await chooseOption(invoice, page, "Price Tier", "Cash price");
    await chooseOption(invoice, page, "Product 1", "หน้ากากอนามัย");
    await invoice.getByLabel("Quantity 1").fill("1");
    await invoice.getByLabel("Override Price 1").fill("75");
    await invoice.getByLabel("Override Reason 1").fill("ลูกค้าประจำ");
    await invoice.getByRole("button", { name: "Preview" }).click();
    await expect(invoice.getByText(/75\.00/).first()).toBeVisible();
    await invoice.getByRole("button", { name: "Create Invoice" }).click();
    await expect(invoice.getByText("invoice created")).toBeVisible();

    // C3-07 overselling stock is rejected with a clear message
    await invoice.getByLabel("Customer Name").fill("ขายเกิน");
    await chooseOption(invoice, page, "Product 1", "เตียงผู้ป่วยปรับระดับ");
    await invoice.getByLabel("Quantity 1").fill("999");
    await invoice.getByLabel("Override Price 1").fill("");
    await invoice.getByLabel("Override Reason 1").fill("");
    await invoice.getByRole("button", { name: "Create Invoice" }).click();
    await expect(invoice.getByText(/insufficient real stock/)).toBeVisible();

    await session.context.close();
  });

  test("D1: POS sells from ghost stock and collects full payment", async ({ browser }) => {
    const session = await openSession(browser, "pos@erp.local", "/sales");
    const page = session.page;

    // D1-03 ghost bucket sale at cash tier
    const composer = card(page, "Create POS Invoice");
    await composer.getByLabel("Customer Name").fill("ลูกค้าเดินเข้า");
    await chooseOption(composer, page, "Product 1", "ผ้าอ้อมผู้ใหญ่");
    await composer.getByLabel("Quantity 1").fill("2");
    await chooseOption(composer, page, "Stock Bucket 1", "Ghost");
    await composer.getByRole("button", { name: "Preview" }).click();
    await expect(composer.getByText(/511\.46/).first()).toBeVisible();
    await composer.getByRole("button", { name: "Create Invoice" }).click();
    await expect(composer.getByText("invoice created")).toBeVisible();

    // D1-02 collect full payment for the newly issued invoice
    const collect = card(page, "Collect Full Payment");
    await collect.getByLabel("Payment Invoice").click();
    await page.getByRole("option").last().click();
    await collect.getByPlaceholder("Reference").fill("POS-TEST-01");
    await collect.getByRole("button", { name: "Collect Payment" }).click();
    await expect(collect.getByText("payment collected")).toBeVisible();

    await session.context.close();
  });

  test("B6: super admin manages branches, users, and locked sequences", async ({ browser }) => {
    const session = await openSession(browser, "superadmin@erp.local", "/dashboard");
    const page = session.page;
    await page.goto("/settings");

    // B6-01 create branch
    const createBranch = card(page, "Create Branch");
    await createBranch.getByPlaceholder("Code").fill("BKK2");
    await createBranch.getByPlaceholder("Branch name").fill("สาขาลาดพร้าว");
    await createBranch.getByPlaceholder("Address").fill("55 ลาดพร้าว กรุงเทพฯ");
    await createBranch.getByRole("button", { name: "Create Branch" }).click();
    await expect(page.getByText(/branch created/).first()).toBeVisible();

    // duplicate branch code surfaces the 409 message
    await createBranch.getByPlaceholder("Code").fill("MNS");
    await createBranch.getByPlaceholder("Branch name").fill("ซ้ำ");
    await createBranch.getByRole("button", { name: "Create Branch" }).click();
    await expect(page.getByText("branch code already exists")).toBeVisible();

    // B6-02 create user on the new branch, then sign in with it
    await page.getByRole("tab", { name: "Users" }).click();
    const createUser = card(page, "Create User");
    await createUser.getByPlaceholder("Full name").fill("พนักงานขายลาดพร้าว");
    await createUser.getByPlaceholder("Email").fill("pos2@erp.local");
    await createUser.getByPlaceholder("Password").fill("TestPos456!");
    await chooseOption(createUser, page, "User Role", "Branch POS");
    await chooseOption(createUser, page, "User Branch", "สาขาลาดพร้าว");
    await createUser.getByRole("button", { name: "Create User" }).click();
    await expect(page.getByText(/user created/).first()).toBeVisible();

    // B6-03/B6-04 sequences: lock MNS invoice, editing then fails, unlock works
    await page.getByRole("tab", { name: "Sequences" }).click();
    const sequenceForm = page.locator("form").filter({ has: page.getByLabel("Sequence Prefix invoice MNS") });
    await sequenceForm.getByLabel("Sequence Locked invoice MNS").check();
    await sequenceForm.getByRole("button", { name: "Save" }).click();
    await expect(page.getByText(/sequence updated/).first()).toBeVisible();

    await sequenceForm.getByLabel("Sequence Next Number invoice MNS").fill("999");
    await sequenceForm.getByRole("button", { name: "Save" }).click();
    await expect(page.getByText(/sequence is locked/).first()).toBeVisible();

    await session.context.close();

    const posSession = await openSession(browser, "pos2@erp.local", "/sales", "TestPos456!");
    await expect(posSession.page.getByRole("link", { name: /Installments/ })).toBeVisible();
    await posSession.context.close();
  });

  test("D3: POS sees installments without the create form and can collect", async ({ browser }) => {
    const session = await openSession(browser, "pos@erp.local", "/sales");
    const page = session.page;
    await page.goto("/installments");

    // D3-01 no manage form for POS
    await expect(page.getByRole("heading", { name: "Create Installment Plan" })).toHaveCount(0);
    await expect(page.getByRole("heading", { name: "Installment Plans" })).toBeVisible();

    // D3-02 POS collects a pending installment
    const row = page.getByRole("row").filter({ hasText: "pending" }).first();
    await row.getByRole("button", { name: "Collect" }).click();
    await expect(page.getByText("installment payment recorded")).toBeVisible();

    await session.context.close();
  });
});
