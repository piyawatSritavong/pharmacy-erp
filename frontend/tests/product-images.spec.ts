import { expect, test } from "@playwright/test";

const password = "DevPassword123!";

async function signIn(page: import("@playwright/test").Page, email: string, path: string) {
  await page.goto("/login");
  await page.getByLabel("อีเมล").fill(email);
  await page.getByLabel("รหัสผ่าน").fill(password);
  await page.getByRole("button", { name: "เข้าสู่ระบบ" }).click();
  await page.waitForURL(new RegExp(`${path}$`));
}

test.describe("รูปสินค้า Ocha", () => {
  test.skip(!process.env.E2E_RUN, "กำหนด E2E_RUN=1 เมื่อเปิดบริการแล้ว");

  test("POS แสดงรูปหลักและหน้าสต๊อกเปิด gallery ได้", async ({ page }) => {
    await signIn(page, "pos.mes@erp.local", "/sales");
    const firstProductImage = page.locator('[data-testid="product-scroll-area"] img').first();
    await expect(firstProductImage).toBeVisible();
    await expect.poll(() => firstProductImage.evaluate((image: HTMLImageElement) => image.naturalWidth)).toBeGreaterThan(0);

    await page.getByRole("button", { name: "ออกจากระบบ" }).click();
    await signIn(page, "superadmin@erp.local", "/dashboard");
    await page.goto("/real-inventory");
    await page.getByLabel("ค้นหาสต๊อกทุกสาขา").fill("ARM SLING Size 1");
    await page.getByRole("button", { name: "ค้นหาและกรอง" }).click();
    await page.getByRole("option").first().click();
    await page.getByRole("button", { name: /ดูรูปทั้งหมด \(2\)/ }).click();
    const gallery = page.getByRole("dialog");
    await expect(gallery.locator("img")).toHaveCount(2);
    for (const image of await gallery.locator("img").all()) {
      await expect.poll(() => image.evaluate((node: HTMLImageElement) => node.naturalWidth)).toBeGreaterThan(0);
    }
  });
});
