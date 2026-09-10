import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

const password = "DevPassword123!";

async function signIn(page: Page, email: string, landing: RegExp) {
  await page.goto("/login");
  await page.getByLabel("อีเมล").fill(email);
  await page.getByLabel("รหัสผ่าน").fill(password);
  await page.getByRole("button", { name: "เข้าสู่ระบบ" }).click();
  await page.waitForURL(landing);
}

/** The branch ids the signed-in caller can see, keyed by branch code. */
async function branchIDs(request: APIRequestContext) {
  const body = await (await request.get("/api/backend/branches")).json();
  const ids: Record<string, string> = {};
  for (const branch of body.items ?? []) ids[branch.code] = branch.id;
  return ids;
}

/**
 * The branch a request acts on comes from the token. A branch id in the query
 * string or the path may narrow within that and never widen it, so a till
 * naming another shop is refused rather than answered.
 *
 * Two of these were reachable: /promotions and a product's images both took
 * whatever branch id arrived and handed back that branch's rows.
 */
test.describe("ขอบเขตสาขา", () => {
  test("หน้าร้านขอข้อมูลของสาขาอื่นด้วยรหัสสาขา ต้องถูกปฏิเสธ", async ({ browser }) => {
    // Head office is the only caller that may enumerate every branch.
    const hq = await browser.newContext();
    const hqPage = await hq.newPage();
    await signIn(hqPage, "superadmin@erp.local", /\/dashboard$/);
    const ids = await branchIDs(hqPage.request);
    await hq.close();

    const shop = await browser.newContext();
    const shopPage = await shop.newPage();
    await signIn(shopPage, "pos.mes@erp.local", /\/sales$/);
    const own = ids.MES;
    const other = ids.KNP;
    expect(own, "ต้องมีสาขา MES").toBeTruthy();
    expect(other, "ต้องมีสาขา KNP").toBeTruthy();

    const product = (await (await shopPage.request.get("/api/backend/products?page_size=1")).json())
      .items?.[0]?.id;
    expect(product, "ต้องมีสินค้าอย่างน้อยหนึ่งรายการ").toBeTruthy();

    for (const path of [
      "promotions?branch_id=",
      "products?branch_id=",
      "aliases?branch_id=",
      "inventory?branch_id=",
      `products/${product}/images?branch_id=`,
      `sales/lot-options?product_id=${product}&stock_bucket=real&branch_id=`,
    ]) {
      // Its own branch answers, so the refusal below is about the branch asked
      // for and not about the endpoint being out of reach anyway.
      expect((await shopPage.request.get(`/api/backend/${path}${own}`)).status(), path).toBeLessThan(400);
      expect((await shopPage.request.get(`/api/backend/${path}${other}`)).status(), path).toBe(403);
    }
    await shop.close();
  });

  test("หน้าร้านขายในนามสาขาอื่นไม่ได้", async ({ browser }) => {
    const hq = await browser.newContext();
    const hqPage = await hq.newPage();
    await signIn(hqPage, "superadmin@erp.local", /\/dashboard$/);
    const ids = await branchIDs(hqPage.request);
    await hq.close();

    const shop = await browser.newContext();
    const shopPage = await shop.newPage();
    await signIn(shopPage, "pos.mes@erp.local", /\/sales$/);

    const line = { quantity: 1, stock_bucket: "real" };
    const refused = await shopPage.request.post("/api/backend/pos/checkout", {
      data: { branch_id: ids.KNP, items: [line], payment_type: "cash", tendered_amount: 1 },
    });
    expect(refused.status()).toBe(403);
    // The branch is settled before the cart is looked at: an empty cart at
    // another shop is turned away as a scope failure, not a bad-request one.
    expect((await refused.json()).message).toContain("สาขาของตนเอง");
    await shop.close();
  });
});
