import { expect, test } from "@playwright/test";

const password = "DevPassword123!";

/** The sidebar is an accordion, so a link is only clickable once its group is
 *  open. Opening the group that already holds the current route would close it. */
async function openFromSidebar(
  page: import("@playwright/test").Page,
  group: string,
  link: string,
) {
  const nav = page.getByRole("navigation", { name: "เมนูหลัก", exact: true });
  const trigger = nav.getByRole("button", { name: group, exact: true });
  if ((await trigger.getAttribute("aria-expanded")) !== "true") {
    await trigger.click();
  }
  await nav.getByRole("link", { name: link, exact: true }).click();
}

test.describe("Supply Chain Management", () => {
  test.skip(!process.env.E2E_RUN, "กำหนด E2E_RUN=1 เมื่อเปิดบริการแล้ว");

  test("สร้างคู่ค้า และรับสินค้าเข้า Lot", async ({
    page,
  }) => {
    const suffix = Date.now();
    const supplierName = `บริษัท SCM E2E ${suffix}`;
    const supplierCode = `SCM-${suffix}`;
    // business-flow.md moved product creation into the catalogue only, so a PO
    // receives an existing product rather than inventing one here.
    let productName = "";
    let supplierID = "";
    let purchaseOrderID = "";

    await page.goto("/login");
    await page.getByLabel("อีเมล").fill("superadmin@erp.local");
    await page.getByLabel("รหัสผ่าน").fill(password);
    await page.getByRole("button", { name: "เข้าสู่ระบบ" }).click();
    await page.waitForURL(/\/dashboard$/);

    await openFromSidebar(page, "ใบเอกสาร", "บริษัทคู่ค้า");
    await expect(
      page.getByRole("heading", { name: "บริษัทคู่ค้า", exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "เพิ่มบริษัทคู่ค้า" }).click();
    const supplierDialog = page.getByRole("dialog");
    await supplierDialog.getByLabel("รหัสคู่ค้า").fill(supplierCode);
    await supplierDialog.getByLabel("ชื่อบริษัท *").fill(supplierName);
    await supplierDialog
      .getByLabel("เลขผู้เสียภาษี")
      .fill(String(suffix).padStart(13, "0").slice(-13));
    await supplierDialog.getByLabel("ที่อยู่").fill("99 ถนนทดสอบ");
    await supplierDialog.getByLabel("จังหวัด").fill("กรุงเทพมหานคร");
    const supplierResponsePromise = page.waitForResponse(
      (response) =>
        response.url().includes("/api/backend/suppliers") &&
        response.request().method() === "POST",
    );
    await supplierDialog
      .getByRole("button", { name: "บันทึก", exact: true })
      .click();
    const supplierResponse = await supplierResponsePromise;
    expect(supplierResponse.status()).toBe(201);
    supplierID = String((await supplierResponse.json()).id);
    // The list is paged, so find the new supplier through search rather than
    // assuming it lands on the first page.
    await page.getByLabel("ค้นหาบริษัทคู่ค้า").fill(supplierName);
    await expect(page.getByText(supplierName, { exact: true }).first()).toBeVisible();

    await openFromSidebar(page, "ใบเอกสาร", "ใบสั่งซื้อเข้า");
    await expect(
      page.getByRole("heading", { name: "ใบสั่งซื้อเข้า", exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "สร้างใบสั่งซื้อเข้า" }).click();
    const poDialog = page.getByRole("dialog");
    await poDialog.getByLabel("บริษัทคู่ค้า *").click();
    await page.getByRole("option", { name: supplierName, exact: true }).click();

    let productPage = 0;
    await page.route(
      "**/api/backend/purchase-orders/product-options?*",
      async (route) => {
        productPage += 1;
        const hasCursor = new URL(route.request().url()).searchParams.has(
          "cursor",
        );
        const items = hasCursor
          ? [
              {
                id: "99999999-9999-4999-8999-999999999999",
                sku: "MOCK-21",
                name: "Mock product 21",
                unit_name: "ชิ้น",
                cost_price: 1,
                sellable_quantity: 0,
              },
            ]
          : Array.from({ length: 20 }, (_, index) => ({
              id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
              sku: `MOCK-${index + 1}`,
              name: `Mock product ${index + 1}`,
              unit_name: "ชิ้น",
              cost_price: 1,
              sellable_quantity: 0,
            }));
        await route.fulfill({
          contentType: "application/json",
          json: {
            items,
            next_cursor: hasCursor ? "" : "MjA",
            has_more: !hasCursor,
          },
        });
      },
    );
    await poDialog.getByLabel("ค้นหาสินค้าเข้า PO").fill("mock");
    const productList = poDialog.getByRole("listbox");
    await expect(
      productList.getByText("Mock product 20", { exact: true }),
    ).toBeVisible();
    await productList.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
      element.dispatchEvent(new Event("scroll"));
    });
    await expect(
      productList.getByText("Mock product 21", { exact: true }),
    ).toBeVisible();
    expect(productPage).toBeGreaterThanOrEqual(2);
    await page.unroute("**/api/backend/purchase-orders/product-options?*");

    await poDialog.getByLabel("ค้นหาสินค้าเข้า PO").fill("OCH-");
    const productOption = productList.getByRole("button").first();
    // Wait for the real catalogue to replace the mocked page above; clicking too
    // early picks a mock id the API does not know.
    await expect(productOption).toContainText("OCH-");
    // Label lines: optional "N รูป", the product name, then "SKU · barcode".
    const optionLines = (await productOption.innerText())
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean);
    const skuLineIndex = optionLines.findIndex((line) => line.startsWith("OCH-"));
    expect(skuLineIndex).toBeGreaterThan(0);
    productName = optionLines[skuLineIndex - 1];
    expect(productName.length).toBeGreaterThan(0);
    await productOption.click();

    await poDialog.getByLabel("จำนวน", { exact: true }).fill("5");
    await poDialog.getByLabel("ราคาซื้อ/หน่วย", { exact: true }).fill("70");
    await poDialog.getByLabel("Lot/Batch", { exact: true }).fill(`BATCH-${suffix}`);
    const expiry = poDialog.getByLabel("วันหมดอายุ *");
    if (await expiry.count()) {
      await expiry.fill("2027-12-31");
    }
    const poResponsePromise = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/backend/purchase-orders") &&
        response.request().method() === "POST",
    );
    await poDialog
      .getByRole("button", { name: "บันทึกและรับเข้าสต๊อก" })
      .click();
    const poResponse = await poResponsePromise;
    expect(poResponse.status()).toBe(201);
    purchaseOrderID = String((await poResponse.json()).id);
    // Saving swaps the create dialog for the read-only detail one; scope to the
    // dialog that owns the detail actions so a stale node cannot match.
    const detailDialog = page.getByRole("dialog").filter({ hasText: "แก้ยอด/หมายเหตุ" });
    // The line shows the product name and its SKU in one cell.
    // The dialog renders a desktop table and a mobile card list, so assert on
    // the dialog's own text instead of picking one of the two nodes.
    await expect(detailDialog).toContainText(productName);
    await expect(detailDialog).toContainText(`BATCH-${suffix}`);
    await expect(detailDialog).toContainText("฿70.00");
    await expect(
      detailDialog.getByRole("button", { name: "แก้รายการ" }),
    ).toBeVisible();
    await expect(
      detailDialog.getByRole("button", { name: "แก้ยอด/หมายเหตุ" }),
    ).toBeVisible();
    await page.keyboard.press("Escape");

    if (purchaseOrderID) {
      const cancel = await page.request.post(
        `/api/backend/purchase-orders/${purchaseOrderID}/cancel`,
        { data: { reason: "ล้างข้อมูลจาก SCM E2E" } },
      );
      expect(cancel.ok()).toBeTruthy();
    }
    if (supplierID) {
      // Deleting a supplier is confirmation-gated: the API wants the exact
      // phrase, the same one the UI makes you type.
      const archive = await page.request.delete(
        `/api/backend/suppliers/${supplierID}`,
        { data: { confirmation: `ลบ ${supplierName}` } },
      );
      expect(archive.ok()).toBeTruthy();
    }
  });
});
