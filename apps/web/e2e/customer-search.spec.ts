import { expect, test } from "@playwright/test";

// End-to-end customer flow: public link → consent → selfie upload → results.
//
// The public search API is mocked at the network boundary so the flow can run
// without a live Go API or indexed data. The web app talks only to
// `/api/v1/public/events/{token}` (GET metadata, POST search), which is exactly
// what a real customer session exercises.
const PUBLIC_TOKEN = "customer-flow-token";

test("customer searches a public Event with a selfie and sees results", async ({ page }) => {
  await page.route("**/api/v1/public/events/**", async (route) => {
    const request = route.request();
    if (request.method() === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ name: "Sự kiện thử nghiệm", expiresAt: null, downloadsEnabled: false }),
      });
      return;
    }
    if (request.method() === "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ results: [{ photoId: "aaaaaaaa-1111" }, { photoId: "bbbbbbbb-2222" }], nextCursor: null }),
      });
      return;
    }
    await route.continue();
  });

  await page.goto(`/e/${PUBLIC_TOKEN}`);
  await expect(page.getByRole("heading", { name: "Sự kiện thử nghiệm" })).toBeVisible();

  const searchButton = page.getByRole("button", { name: "Tìm ảnh" });
  await page.setInputFiles("#selfie", {
    name: "selfie.jpg",
    mimeType: "image/jpeg",
    buffer: Buffer.from("selfie-bytes"),
  });
  await expect(searchButton).toBeDisabled();

  await page.getByRole("checkbox").check();
  await expect(searchButton).toBeEnabled();

  await searchButton.click();
  await expect(page.getByText("Tìm thấy 2 ảnh")).toBeVisible();
  await expect(page.getByRole("listitem")).toHaveCount(2);
});
