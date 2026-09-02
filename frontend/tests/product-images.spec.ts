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

    // Pick a product that actually carries images instead of naming one: the
    // option label starts with its image count ("2 รูป ... SKU · สาขา ...").
    const options = page.getByRole("option");
    await expect(options.first()).toBeVisible();
    const labels = await options.allInnerTexts();
    const withImages = labels
      .map((label) => label.replace(/\s+/g, " ").trim())
      .find((label) => /^[1-9]\d* รูป /.test(label));
    expect(withImages, "the stock list must offer a product with images").toBeTruthy();
    const sku = withImages!.match(/\b[A-Z]{2,}-[A-Z0-9]+\b/)![0];

    // Searching narrows the same list; there is no separate search button.
    await page.getByLabel("ค้นหาสต๊อกทุกสาขา").fill(sku);
    await expect.poll(async () => (await options.first().innerText()).includes(sku)).toBe(true);
    await options.first().click();

    const gallery = page.locator('[data-testid="product-image-swiper"]');
    const shown = gallery.locator("img").first();
    await expect(shown).toBeVisible();
    await expect.poll(() => shown.evaluate((node: HTMLImageElement) => node.naturalWidth)).toBeGreaterThan(0);

    // Every image in the swiper has to decode, not just the one on top.
    const dots = gallery.getByRole("button", { name: /^รูปที่ \d+$/ });
    const imageCount = await dots.count();
    expect(imageCount).toBeGreaterThan(1);
    for (let index = 1; index < imageCount; index += 1) {
      await dots.nth(index).click();
      await expect.poll(() => shown.evaluate((node: HTMLImageElement) => node.naturalWidth)).toBeGreaterThan(0);
    }
  });
});
