import { expect, test, type Page } from "@playwright/test";

const password = "DevPassword123!";

async function signIn(page: Page, email: string, landing: RegExp) {
  await page.goto("/login");
  await page.getByLabel("อีเมล").fill(email);
  await page.getByLabel("รหัสผ่าน").fill(password);
  await page.getByRole("button", { name: "เข้าสู่ระบบ" }).click();
  await page.waitForURL(landing);
}

/**
 * Fills the promotion form and submits it. The suite creates whatever it needs
 * rather than leaning on rows someone left in the database, so it says the same
 * thing on a freshly seeded system as on a used one.
 */
async function pick(page: Page, label: string, option: string | RegExp) {
  // The selects are Radix comboboxes, so they open and are picked from rather
  // than set like a native <select>.
  await page.getByRole("combobox", { name: label }).click();
  await page.getByRole("option", { name: option }).first().click();
}

async function createPercentPromotion(page: Page, code: string, name: string, branchOption?: string | RegExp) {
  await page.goto("/promotions");
  await pick(page, "ชนิดโปรโมชั่น", "ลดเป็นเปอร์เซ็นต์");
  await page.getByLabel("รหัสโปรโมชั่น").fill(code);
  await page.getByLabel("ชื่อโปรโมชั่น").fill(name);
  if (branchOption !== undefined) {
    await pick(page, "สาขาโปรโมชั่น", branchOption);
  }
  await page.getByLabel("ส่วนลดเปอร์เซ็นต์").fill("10");
  await pick(page, "สินค้า 1", /.+/);
  await page.getByRole("button", { name: "สร้างโปรโมชั่น" }).click();
  await expect(page.getByText(/สร้างโปรโมชั่นแล้ว|รหัสโปรโมชั่นนี้ถูกใช้แล้ว/)).toBeVisible();
}

test.describe("โปรโมชั่นของแต่ละสาขา", () => {
  test("หน้าร้านมีเมนูโปรโมชั่น และถูกผูกกับสาขาของตัวเอง", async ({ page }) => {
    await signIn(page, "pos.mes@erp.local", /\/sales$/);
    await expect(page.getByRole("link", { name: /โปรโมชั่น/ })).toBeVisible();

    await page.goto("/promotions");
    await expect(page.getByRole("heading", { name: "โปรโมชั่น", exact: true }).first()).toBeVisible();
    // The shop is told which branch it is setting up for rather than asked: the
    // server pins the write to whoever is signed in, so a picker would only
    // offer choices the server would refuse.
    const branchField = page.getByLabel("สาขาโปรโมชั่น");
    await expect(branchField).toBeDisabled();
    await expect(branchField).toHaveValue("MES");
    await expect(page.getByText("โปรโมชั่นนี้ใช้ที่สาขานี้เท่านั้น")).toBeVisible();
  });

  test("สาขาแก้โปรของตัวเองได้ แต่ของสำนักงานใหญ่อ่านได้อย่างเดียว", async ({ browser }) => {
    // Head office sets one for every branch...
    const hqContext = await browser.newContext();
    const hqPage = await hqContext.newPage();
    await signIn(hqPage, "superadmin@erp.local", /\/dashboard$/);
    await createPercentPromotion(hqPage, "E2EALL", "อีทูอี ทุกสาขา");
    await hqContext.close();

    // ...and the shop sets its own. It sees both, and may remove only its own.
    const shopContext = await browser.newContext();
    const shopPage = await shopContext.newPage();
    await signIn(shopPage, "pos.mes@erp.local", /\/sales$/);
    await createPercentPromotion(shopPage, "E2EMES", "อีทูอี เฉพาะ MES");

    await expect(shopPage.getByText("อีทูอี ทุกสาขา")).toBeVisible();
    await expect(shopPage.getByText("ตั้งจากสำนักงานใหญ่").first()).toBeVisible();
    await expect(shopPage.getByRole("button", { name: /^ลบโปรโมชั่น อีทูอี ทุกสาขา/ })).toHaveCount(0);
    await expect(shopPage.getByRole("button", { name: /^ลบโปรโมชั่น อีทูอี เฉพาะ MES/ })).toBeVisible();
    await shopContext.close();
  });

  test("อีกสาขาไม่เห็นโปรของสาขาแรก และใช้รหัสเดียวกันได้", async ({ page }) => {
    await signIn(page, "pos.knp@erp.local", /\/sales$/);
    await page.goto("/promotions");
    await expect(page.getByText("อีทูอี เฉพาะ MES")).toHaveCount(0);
    // The same code again, at a different shop — independent namespaces.
    await createPercentPromotion(page, "E2EMES", "อีทูอี เฉพาะ คณาเภสัช");
    await expect(page.getByText("อีทูอี เฉพาะ คณาเภสัช")).toBeVisible();
  });

  test("สำนักงานใหญ่ยังเลือกได้ว่าจะให้สาขาไหน หรือทุกสาขา", async ({ page }) => {
    await signIn(page, "superadmin@erp.local", /\/dashboard$/);
    await page.goto("/promotions");
    await expect(page.getByRole("combobox", { name: "สาขาโปรโมชั่น" })).toBeEnabled();
    await expect(page.getByText("ตั้งจากสำนักงานใหญ่")).toHaveCount(0);
  });
});
