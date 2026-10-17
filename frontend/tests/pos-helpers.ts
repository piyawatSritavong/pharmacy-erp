import { expect, type Locator, type Page } from "@playwright/test";

/**
 * Taps a product's add button and finishes the add. A product with one lot
 * goes straight into the cart; one with several asks which lot first.
 */
export async function addProductToCart(page: Page, button: Locator) {
  await button.click();
  const lotDialog = page.getByRole("dialog", { name: /^เลือก Lot/ });
  const added = page.getByRole("status").filter({ hasText: /^เพิ่ม .* แล้ว$/ }).first();
  await expect(lotDialog.or(added).first()).toBeVisible();
  if (await lotDialog.isVisible()) {
    await lotDialog.getByRole("button", { name: /^Lot / }).first().click();
    await expect(lotDialog).toHaveCount(0);
  }
}

/** Picks a payment method in the payment dialog. */
export async function choosePaymentMethod(payment: Locator, method: string | RegExp) {
  await payment.getByRole("radio", { name: method, exact: typeof method === "string" }).click();
}
