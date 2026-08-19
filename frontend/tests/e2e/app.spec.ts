import { expect, test } from '@playwright/test';
import { demoSnapshot } from '../../src/app/mock-data';

test('renders the FakeMex terminal shell', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'FakeMex' })).toBeVisible();
  await expect(page.getByLabel('Theme selector')).toBeVisible();
});

test('refresh frequency slider updates live and persists', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto('/');

  const slider = page.getByLabel('Recovery frequency');
  const indicator = page.locator('.refresh-meta .refresh-value');

  await expect(slider).toBeVisible();
  await expect(indicator).toHaveText('1000ms');

  await slider.evaluate((el, target) => {
    (el as HTMLInputElement).value = String(target);
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
  }, 750);

  await expect(indicator).toHaveText('750ms');
  const stored = await page.evaluate(() => localStorage.getItem('fakemex-refresh-interval-ms'));
  expect(stored).toBe('750');

  await page.reload();
  await expect(indicator).toHaveText('750ms');
  const restored = await page.evaluate(() => localStorage.getItem('fakemex-refresh-interval-ms'));
  expect(restored).toBe('750');
});

test('theme search accepts Catppuccin Mocha misspelling and persists', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto('/');

  const themeInput = page.getByRole('combobox', { name: 'Theme selector' });
  await themeInput.fill('catpuccino');
  await themeInput.press('Enter');
  await page.getByRole('button', { name: 'Apply' }).click();

  await expect(themeInput).toHaveValue('Catppuccin Mocha');
  const persistedTheme = await page.evaluate(() => localStorage.getItem('fakemex-kitty-theme'));
  expect(persistedTheme).toBe('catppuccin-mocha');

  const shellBg = await page.evaluate(() =>
    getComputedStyle(document.documentElement)
      .getPropertyValue('--fakemex-shell-bg')
      .trim()
      .toLowerCase(),
  );
  expect(shellBg).toBe('#1e1e2e');
});

test('shows instrument selector and chart panel in desktop mode', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto('/');
  await expect(page.getByLabel('Instrument')).toBeVisible();
  await expect(page.locator('.toolbar-field .mat-mdc-select-value-text')).toContainText(
    'BTC / USD',
  );
  await expect(page.locator('.grid-stack')).toBeVisible();
  await expect(page.locator('.grid-stack-item')).toHaveCount(14);
});

test('order form expresses size and price units with BTC/USD', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto('/');

  const orderPanel = page.locator('.grid-stack-item[gs-id="order-form"]');
  const sizeLabel = orderPanel.locator('.size-field .mdc-floating-label');
  const limitPriceLabel = orderPanel.locator('.price-field .mdc-floating-label');
  await expect(sizeLabel).toHaveText('Size (BTC)');
  await expect(limitPriceLabel).toHaveText('Limit price (USD)');

  await orderPanel.locator('.size-field input').fill('2');
  await orderPanel.locator('.price-field input').fill('100');
  await expect(orderPanel.locator('.order-notional')).toHaveText('Est. notional 200 USD');
});

test('styles position symbols, sides, and PnL by meaning', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto('/');

  const cells = page.locator('.grid-stack-item[gs-id="positions"] tbody tr').first().locator('td');
  const symbol = cells.nth(0);
  const side = cells.nth(1);
  const pnl = cells.nth(5);

  await expect(symbol).toHaveCSS('font-weight', '700');
  const sideValue = (await side.textContent())?.trim().toLowerCase();
  expect(sideValue === 'buy' || sideValue === 'sell').toBe(true);
  await expect(side).toHaveClass(sideValue === 'buy' ? /cell-buy/ : /cell-sell/);

  const pnlValue = Number((await pnl.textContent())?.replace(/[,\s%$]/g, ''));
  expect(Number.isFinite(pnlValue) && pnlValue !== 0).toBe(true);
  await expect(pnl).toHaveClass(pnlValue > 0 ? /cell-positive/ : /cell-negative/);

  const colors = await page.evaluate(() => {
    const probe = document.createElement('span');
    const rootStyle = getComputedStyle(document.documentElement);
    probe.style.color = rootStyle.getPropertyValue('--fakemex-buy');
    document.body.appendChild(probe);
    const buy = getComputedStyle(probe).color;
    probe.style.color = rootStyle.getPropertyValue('--fakemex-sell');
    const sell = getComputedStyle(probe).color;
    probe.remove();
    return { buy, sell };
  });
  await expect(side).toHaveCSS('color', sideValue === 'buy' ? colors.buy : colors.sell);
  await expect(pnl).toHaveCSS('color', pnlValue > 0 ? colors.buy : colors.sell);
});

test('renders the selected position on the chart and closes the table row symbol', async ({
  page,
}) => {
  let closePath = '';
  let closePayload: unknown;
  await page.route('**/api/v1/bootstrap**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(demoSnapshot) });
  });
  await page.route('**/api/v1/trading', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ available: true, enabled: true, network: 'testnet' }),
    });
  });
  await page.route('**/api/v1/positions/*/close', async (route) => {
    closePath = new URL(route.request().url()).pathname;
    closePayload = route.request().postDataJSON();
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        requestId: 'e2e-close',
        status: 'ok',
        filled: '0.35',
        averagePrice: '62480.10',
      }),
    });
  });

  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto('/');

  const chart = page.locator('.grid-stack-item[gs-id="chart"]');
  await expect(chart.locator('.position-pill')).toContainText('BUY 0.35 @ 61234.12');
  await expect(chart.locator('.position-pill .pnl-positive')).toHaveText('419.42');

  const positions = page.locator('.grid-stack-item[gs-id="positions"]');
  await expect(positions.getByRole('columnheader', { name: 'Action' })).toBeVisible();
  await positions.getByRole('button', { name: 'Close BTC position' }).click();

  await expect.poll(() => closePath).toBe('/api/v1/positions/BTC/close');
  expect(closePayload).toEqual({ percent: 100, kind: 'market' });
  await expect(page.locator('.trade-submitted')).toContainText(
    'Position close submitted for BTC at 62480.10',
  );
});

test('sorts table columns from their headers and uses bold data rows', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.route('**/api/v1/bootstrap**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(demoSnapshot) });
  });
  await page.goto('/');

  const fills = page.locator('.grid-stack-item[gs-id="fills"]');
  const priceHeader = fills.getByRole('button', { name: 'Sort Px ascending' });
  await priceHeader.click();

  const prices = fills.locator('tbody tr td:nth-child(4)');
  await expect(prices.nth(1)).toBeVisible();
  const ascendingPrices = (await prices.allTextContents()).map((value) =>
    Number(value.replace(/,/g, '')),
  );
  expect(ascendingPrices.length).toBeGreaterThan(1);
  expect(ascendingPrices).toEqual([...ascendingPrices].sort((left, right) => left - right));
  await expect(prices.nth(0)).toHaveCSS('font-weight', '700');

  await fills.getByRole('button', { name: 'Sort Px descending' }).click();
  await expect
    .poll(async () =>
      (await prices.allTextContents()).map((value) => Number(value.replace(/,/g, ''))),
    )
    .toEqual([...ascendingPrices].sort((left, right) => right - left));

  const orderBook = page.locator('.grid-stack-item[gs-id="book"]');
  await orderBook.getByRole('button', { name: 'Sort Size ascending' }).click();
  const sizes = orderBook.locator('tbody tr td:nth-child(2)');
  await expect(sizes.nth(1)).toBeVisible();
  const expectedSizes = (await sizes.allTextContents())
    .map((value) => Number(value.replace(/,/g, '')))
    .sort((left, right) => left - right);
  await expect
    .poll(async () =>
      (await sizes.allTextContents()).map((value) => Number(value.replace(/,/g, ''))),
    )
    .toEqual(expectedSizes);
  const ascendingSizes = (await sizes.allTextContents()).map((value) =>
    Number(value.replace(/,/g, '')),
  );
  expect(ascendingSizes.length).toBeGreaterThan(1);
  expect(ascendingSizes).toEqual([...ascendingSizes].sort((left, right) => left - right));
  await expect(sizes.first()).toHaveCSS('font-weight', '700');
});

test('notifies with actual fill price and realized slippage after submission', async ({ page }) => {
  await page.route('**/api/v1/trading', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ available: true, enabled: true, network: 'testnet' }),
    });
  });
  await page.route('**/api/v1/orders', async (route) => {
    if (route.request().method() !== 'POST') {
      await route.continue();
      return;
    }
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        requestId: 'e2e-fill',
        status: 'ok',
        orderId: '42',
        filled: '0.01',
        averagePrice: '50125',
      }),
    });
  });

  await page.goto('/');
  await expect(page.getByLabel('Disable trading')).toBeVisible();
  await page.getByLabel('Size (BTC)').fill('0.01');
  await page.getByLabel('Limit price (USD)').fill('50000');
  await page.getByRole('button', { name: 'Submit Order' }).click();

  const notification = page.locator('.trade-submitted');
  await expect(notification).toContainText('Filled at 50,125 USD');
  await expect(notification).toContainText('Slippage +0.2500%');
});

test('warns and does not send an order while trading is disabled', async ({ page }) => {
  let orderPosts = 0;
  await page.route('**/api/v1/trading', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ available: true, enabled: false, network: 'testnet' }),
    });
  });
  page.on('request', (request) => {
    if (request.method() === 'POST' && request.url().endsWith('/api/v1/orders')) {
      orderPosts += 1;
    }
  });

  await page.goto('/');
  await page.getByLabel('Size (BTC)').fill('0.01');
  await page.getByLabel('Limit price (USD)').fill('50000');
  await page.getByRole('button', { name: 'Submit Order' }).click();

  await expect(page.getByText(/Trading is unavailable|Trading is disabled/)).toBeVisible();
  expect(orderPosts).toBe(0);
});

test('switches to mainnet through the backend and visibly disarms trading', async ({ page }) => {
  await page.route('**/api/v1/bootstrap**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(demoSnapshot) });
  });
  await page.route('**/api/v1/trading', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ available: true, enabled: true, network: 'testnet' }),
    });
  });
  await page.route('**/api/v1/network', async (route) => {
    expect(route.request().method()).toBe('PUT');
    expect(route.request().postDataJSON()).toEqual({ network: 'mainnet' });
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        network: 'mainnet',
        availableNetworks: ['testnet', 'mainnet'],
        tradingAvailable: true,
        tradingEnabled: false,
      }),
    });
  });

  await page.goto('/');
  await expect(page.getByLabel('Enable Mainnet')).toBeVisible();
  await expect(page.locator('.network-status-badge')).toHaveText('TESTNET');
  await expect(page.getByLabel('Disable trading')).toBeVisible();
  const themeColors = await page.evaluate(() => {
    const resolve = (token: string) => {
      const probe = document.createElement('span');
      probe.style.color = getComputedStyle(document.documentElement).getPropertyValue(token);
      document.body.appendChild(probe);
      const value = getComputedStyle(probe).color;
      probe.remove();
      return value;
    };
    return {
      text: resolve('--fakemex-text'),
      buy: resolve('--fakemex-buy'),
      sell: resolve('--fakemex-sell'),
    };
  });
  await expect(page.locator('.network-toggle .mdc-label')).toHaveCSS('color', themeColors.text);
  await expect(page.locator('.trading-toggle .mdc-label')).toHaveCSS('color', themeColors.text);
  await expect(page.locator('.trading-toggle .mdc-switch__icon--on')).toHaveCSS(
    'fill',
    themeColors.buy,
  );
  await page.getByLabel('Enable Mainnet').click();

  await expect(page.getByLabel('Disable Mainnet')).toBeVisible();
  await expect(page.locator('.network-status-badge')).toHaveText('MAINNET');
  await expect(page.locator('.network-toggle .mdc-label')).toHaveCSS('color', themeColors.sell);
  await expect(page.locator('.network-toggle .mdc-switch__icon--on')).toHaveCSS(
    'fill',
    themeColors.buy,
  );
  await expect(page.getByLabel('Enable trading')).toBeVisible();
  await expect(page.getByText(/MAINNET selected.*Trading was disabled/)).toBeVisible();
});

test('rejects mainnet when its signed local configuration is incomplete', async ({ page }) => {
  await page.route('**/api/v1/bootstrap**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(demoSnapshot) });
  });
  await page.route('**/api/v1/trading', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ available: true, enabled: false, network: 'testnet' }),
    });
  });
  await page.route('**/api/v1/network', async (route) => {
    await route.fulfill({
      status: 412,
      contentType: 'application/problem+json',
      body: JSON.stringify({
        type: 'https://fakemex.local/problems/network',
        title: 'Mainnet unavailable',
        status: 412,
        detail:
          'mainnet requires complete HL_MAINNET_ACCOUNT_ADDRESS, HL_MAINNET_API_WALLET_ADDRESS, and HL_MAINNET_API_WALLET_PRIVATE_KEY configuration',
        code: 'network_unavailable',
      }),
    });
  });

  await page.goto('/');
  await page.getByLabel('Enable Mainnet').click();

  await expect(
    page.getByText(/mainnet requires complete HL_MAINNET_ACCOUNT_ADDRESS/),
  ).toBeVisible();
  await expect(page.getByLabel('Enable Mainnet')).toBeVisible();
  await expect(page.locator('.network-status-badge')).toHaveText('TESTNET');
});

test('shows margin mode badge beside stream status and theme Apply aligns with strip controls', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto('/');

  const status = page.locator('.toolbar .status-inline').first();
  const networkBadge = page.locator('.toolbar .network-status-badge');
  const marginBadge = page.locator('.toolbar .margin-mode-badge:not(.network-status-badge)');
  await expect(status).toBeVisible();
  await expect(networkBadge).toBeVisible();
  await expect(marginBadge).toBeVisible();
  await expect(networkBadge).toHaveText(/\b(TESTNET|MAINNET)\b/);
  await expect(marginBadge).toHaveText(/\b(CROSS|ISOLATED)\b/);
  await expect(page.locator('.toolbar .status.status-inline + .network-status-badge')).toHaveCount(
    1,
  );
  await expect(page.locator('.toolbar .network-status-badge + .margin-mode-badge')).toHaveCount(1);
  const statusBox = await status.boundingBox();
  const networkBox = await networkBadge.boundingBox();
  const badgeBox = await marginBadge.boundingBox();
  expect(statusBox).not.toBeNull();
  expect(networkBox).not.toBeNull();
  expect(badgeBox).not.toBeNull();
  expect(networkBox!.x).toBeGreaterThan(statusBox!.x);
  expect(badgeBox!.x).toBeGreaterThan(networkBox!.x);
  expect(
    Math.abs(badgeBox!.y + badgeBox!.height / 2 - (statusBox!.y + statusBox!.height / 2)),
  ).toBeLessThan(8);
  expect(await networkBadge.getAttribute('aria-label')).toMatch(/Network (testnet|mainnet)/);
  expect(await marginBadge.getAttribute('aria-label')).toMatch(/Margin mode (CROSS|ISOLATED)/);

  const presetControl = page.locator('.control-strip .strip-control').nth(0);
  const densityControl = page.locator('.control-strip .strip-control').nth(1);
  const applyButton = page.getByRole('button', { name: 'Apply' }).first();
  const presetBox = await presetControl.boundingBox();
  const densityBox = await densityControl.boundingBox();
  const applyBox = await applyButton.boundingBox();
  expect(presetBox).not.toBeNull();
  expect(densityBox).not.toBeNull();
  expect(applyBox).not.toBeNull();

  const heightTolerance = 3;
  expect(Math.abs(applyBox!.height - presetBox!.height)).toBeLessThanOrEqual(heightTolerance);
  expect(Math.abs(applyBox!.height - densityBox!.height)).toBeLessThanOrEqual(heightTolerance);
  expect(Math.abs(applyBox!.width - presetBox!.width)).toBeLessThanOrEqual(30);
});

test('rectangular elements use unified radius while circular controls stay circular', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto('/');

  const samples = await page.evaluate(() => {
    const pick = (selector: string) => {
      const el = document.querySelector(selector) as Element | null;
      if (!el) return '';
      const radius = getComputedStyle(el).borderRadius;
      return radius.split(' ')[0].trim();
    };

    return {
      topbar: pick('.topbar'),
      controlStrip: pick('.control-strip'),
      panel: pick('.fm-panel'),
      chartInterval: pick('.chart-toolbar .interval'),
      themeApply: pick('.theme-apply'),
      marginBadge: pick('.margin-mode-badge'),
      dot: pick('.connection-toast .dot'),
      resizeGrip: pick('.grid-stack .ui-resizable-se'),
    };
  });

  const rectangular = [
    samples.topbar,
    samples.controlStrip,
    samples.panel,
    samples.chartInterval,
    samples.themeApply,
    samples.marginBadge,
  ].filter((item) => item);

  const uniq = new Set(rectangular);
  expect(uniq.size).toBe(1);
  expect(uniq.has('8px') || uniq.has('0.5rem')).toBeTruthy();

  expect(samples.dot).toMatch(/(^50%$|^999px$)/);
  if (samples.resizeGrip && samples.resizeGrip !== '0px') {
    expect(samples.resizeGrip).toMatch(/(^50%$|^999px$)/);
  }
});

test('keeps chart axes interactive and resizes the tile only from its corner grip', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto('/');

  const panel = page.locator('.grid-stack-item[gs-id="chart"]').first();
  await expect(panel).toBeVisible();
  await expect(panel.locator('.ui-resizable-e, .ui-resizable-s')).toHaveCount(0);

  const chartCanvases = panel.locator('.chart-root canvas');
  const priceAxis = chartCanvases.nth(2);
  const timeAxis = chartCanvases.nth(4);
  await priceAxis.evaluate((element) => element.scrollIntoView({ block: 'center' }));

  const beforeAxisDrag = await panel.boundingBox();
  const priceAxisBox = await priceAxis.boundingBox();
  expect(beforeAxisDrag).not.toBeNull();
  expect(priceAxisBox).not.toBeNull();
  await page.mouse.move(
    priceAxisBox!.x + priceAxisBox!.width / 2,
    priceAxisBox!.y + priceAxisBox!.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    priceAxisBox!.x + priceAxisBox!.width / 2,
    priceAxisBox!.y + priceAxisBox!.height / 2 + 160,
    { steps: 10 },
  );
  await page.mouse.up();
  await page.waitForTimeout(200);

  const afterPriceAxisDrag = await panel.boundingBox();
  expect(afterPriceAxisDrag).not.toBeNull();
  expect(Math.abs(afterPriceAxisDrag!.x - beforeAxisDrag!.x)).toBeLessThan(1);
  expect(Math.abs(afterPriceAxisDrag!.y - beforeAxisDrag!.y)).toBeLessThan(1);

  const timeAxisBox = await timeAxis.boundingBox();
  expect(timeAxisBox).not.toBeNull();
  await page.mouse.move(
    timeAxisBox!.x + timeAxisBox!.width / 2,
    timeAxisBox!.y + timeAxisBox!.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    timeAxisBox!.x + timeAxisBox!.width / 2 + 160,
    timeAxisBox!.y + timeAxisBox!.height / 2,
    { steps: 10 },
  );
  await page.mouse.up();
  await page.waitForTimeout(200);
  const afterTimeAxisDrag = await panel.boundingBox();
  expect(afterTimeAxisDrag).not.toBeNull();
  expect(Math.abs(afterTimeAxisDrag!.x - afterPriceAxisDrag!.x)).toBeLessThan(1);
  expect(Math.abs(afterTimeAxisDrag!.y - afterPriceAxisDrag!.y)).toBeLessThan(1);

  const resizeHandle = panel.locator('.ui-resizable-se');
  await expect(resizeHandle).toBeVisible();

  const dragSouth = async (deltaY: number) => {
    await resizeHandle.evaluate((el) => el.scrollIntoView({ block: 'center', inline: 'center' }));
    const handleBox = await resizeHandle.boundingBox();
    expect(handleBox).not.toBeNull();
    const x = handleBox!.x + handleBox!.width / 2;
    const y = handleBox!.y + handleBox!.height / 2;
    await page.mouse.move(x, y);
    await page.mouse.down();
    await page.mouse.move(x, y + deltaY, { steps: 10 });
    await page.mouse.up();
    await page.waitForTimeout(120);
  };

  await resizeHandle.evaluate((element) => element.scrollIntoView({ block: 'center' }));
  const before = await panel.boundingBox();
  expect(before).not.toBeNull();
  await dragSouth(-180);

  const shrunken = await panel.boundingBox();
  expect(shrunken).not.toBeNull();
  expect(shrunken!.height).toBeLessThan(before!.height - 20);

  await dragSouth(220);

  const expanded = await panel.boundingBox();
  expect(expanded).not.toBeNull();
  expect(expanded!.height).toBeGreaterThan(before!.height - 20);
  expect(expanded!.height).toBeGreaterThan(shrunken!.height + 20);
});

test('captures dense viewport and chart sanity for full desktop terminal', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto('/');

  await expect(page.locator('.topbar')).toBeVisible();
  await expect(page.getByLabel('Instrument')).toBeVisible();
  await expect(page.locator('.grid-stack')).toBeVisible();
  await expect(page.locator('.chart-toolbar')).toBeVisible();
  await expect(page.locator('app-market-chart')).toBeVisible();
  await expect(page.locator('app-market-chart .chart-root')).toBeVisible();
  const chartCanvas = page.locator('app-market-chart .chart-root canvas').first();
  const depthCanvas = page.locator('app-depth-chart .depth-root canvas').first();
  await expect(chartCanvas).toBeVisible();
  await expect(depthCanvas).toBeVisible();
  const chartSize = await chartCanvas.boundingBox();
  const depthSize = await depthCanvas.boundingBox();
  expect(chartSize?.width ?? 0).toBeGreaterThan(20);
  expect(chartSize?.height ?? 0).toBeGreaterThan(20);
  expect(depthSize?.width ?? 0).toBeGreaterThan(20);
  expect(depthSize?.height ?? 0).toBeGreaterThan(20);
  await expect(page.locator('app-depth-chart .depth-root')).toBeVisible();
  await expect(depthCanvas).toBeVisible();

  const headerStatus = (await page.locator('.toolbar .status.status-inline').innerText())
    .trim()
    .toLowerCase();
  const toastText = (await page.locator('.connection-toast > span').nth(1).innerText())
    .trim()
    .toLowerCase();
  const chartStatus = (await page.locator('app-market-chart .status-pill').first().innerText())
    .trim()
    .toLowerCase();
  if (headerStatus.includes('demo')) {
    expect(chartStatus).toBe('demo');
    expect(toastText).toContain('demo');
  } else if (headerStatus.includes('reconnecting')) {
    expect(['demo', 'offline']).toContain(chartStatus);
    expect(toastText).toContain('reconnect');
  } else if (headerStatus.includes('offline')) {
    expect(chartStatus).toBe('offline');
    expect(toastText).toContain('unavailable');
  } else {
    expect(chartStatus).toBe('live');
  }

  const bodyHeight = await page.evaluate(() => document.body.scrollHeight);
  expect(bodyHeight).toBeGreaterThan(1600);
  expect(bodyHeight).toBeLessThan(2800);
  const settingsPanel = page.getByRole('heading', { name: 'Settings' });
  await settingsPanel.scrollIntoViewIfNeeded();
  await expect(settingsPanel).toBeVisible();
  const scrollPosition = await page.evaluate(() => window.scrollY);
  expect(scrollPosition).toBeGreaterThan(0);
  const settingsBox = await settingsPanel.boundingBox();
  expect(settingsBox).not.toBeNull();
  expect(settingsBox!.y).toBeGreaterThanOrEqual(0);
  expect(settingsBox!.y).toBeLessThan(900);

  const contrast = await page.evaluate(() => {
    const get = (name: string) =>
      getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    const parse = (value: string) => {
      const normalized = value.replace('#', '');
      if (normalized.length !== 6) return [0, 0, 0];
      const n = Number.parseInt(normalized, 16);
      return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
    };
    const luminance = ([r, g, b]: number[]) => {
      const v = ([r, g, b] as number[]).map((channel) => {
        const scaled = channel / 255;
        return scaled <= 0.03928 ? scaled / 12.92 : Math.pow((scaled + 0.055) / 1.055, 2.4);
      });
      return 0.2126 * v[0] + 0.7152 * v[1] + 0.0722 * v[2];
    };
    const ratio = (fg: string, bg: string) => {
      const l1 = luminance(parse(fg));
      const l2 = luminance(parse(bg));
      const bright = Math.max(l1, l2);
      const dim = Math.min(l1, l2);
      return (bright + 0.05) / (dim + 0.05);
    };

    return {
      text: ratio(get('--fakemex-text'), get('--fakemex-shell-bg')),
      panelText: ratio(get('--fakemex-text'), get('--fakemex-panel-body')),
      muted: ratio(get('--fakemex-muted'), get('--fakemex-panel-body')),
      inputText: ratio(get('--fakemex-input-text'), get('--fakemex-input-bg')),
    };
  });
  expect(contrast.text).toBeGreaterThan(4);
  expect(contrast.panelText).toBeGreaterThan(4);
  expect(contrast.muted).toBeGreaterThan(2.6);
  expect(contrast.inputText).toBeGreaterThan(4);

  const renderedControlColors = await page
    .locator('.toolbar-field .mat-mdc-select-value-text')
    .evaluate((element) => {
      const expectedToken = getComputedStyle(document.documentElement)
        .getPropertyValue('--fakemex-input-text')
        .trim();
      const probe = document.createElement('span');
      probe.style.color = expectedToken;
      document.body.appendChild(probe);
      const expected = getComputedStyle(probe).color;
      probe.remove();
      return {
        actual: getComputedStyle(element).color,
        expected,
      };
    });
  expect(renderedControlColors.actual).toBe(renderedControlColors.expected);
  const toolbarButtonColor = await page
    .getByRole('button', { name: 'Refresh' })
    .evaluate((element) => {
      const token = getComputedStyle(document.documentElement)
        .getPropertyValue('--fakemex-text')
        .trim();
      const probe = document.createElement('span');
      probe.style.color = token;
      document.body.appendChild(probe);
      const expected = getComputedStyle(probe).color;
      probe.remove();
      return { actual: getComputedStyle(element).color, expected };
    });
  expect(toolbarButtonColor.actual).toBe(toolbarButtonColor.expected);

  await page.screenshot({
    path: 'test-artifacts/terminal-full-v2.png',
    fullPage: true,
  });

  const themeInput = page.getByRole('combobox', { name: 'Theme selector' });
  await themeInput.fill('Catppuccin Latte');
  await themeInput.press('Enter');
  await page.getByRole('button', { name: 'Apply' }).click();
  await themeInput.blur();
  await page.waitForTimeout(350);
  const lightShell = await page.evaluate(() =>
    getComputedStyle(document.documentElement).getPropertyValue('--fakemex-shell-bg').trim(),
  );
  expect(lightShell.toLowerCase()).toContain('eff1f5');
  await page.screenshot({
    path: 'test-artifacts/terminal-full-v2-light.png',
    fullPage: true,
  });

  const storedTheme = await page.evaluate(() => localStorage.getItem('fakemex-kitty-theme'));
  expect(storedTheme).toBe('catppuccin-latte');
});

test('mobile layout remains usable and captures mobile terminal state', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 900 });
  await page.goto('/');

  await expect(page.locator('.mobile-tabs')).toBeVisible();
  await expect(page.locator('.mobile-panel')).toBeVisible();
  const topbar = await page.locator('.topbar').boundingBox();
  const connection = await page.locator('.connection-toast').boundingBox();
  const tabs = await page.locator('.mobile-tabs').boundingBox();
  const panel = await page.locator('.mobile-panel').boundingBox();
  expect(topbar).not.toBeNull();
  expect(connection).not.toBeNull();
  expect(tabs).not.toBeNull();
  expect(panel).not.toBeNull();
  expect(connection!.y).toBeGreaterThanOrEqual(topbar!.y + topbar!.height - 1);
  expect(tabs!.y).toBeGreaterThanOrEqual(connection!.y + connection!.height - 1);
  expect(panel!.y).toBeGreaterThanOrEqual(tabs!.y + tabs!.height - 1);
  const inactiveTabColor = await page
    .getByRole('tab', { name: 'Order entry' })
    .evaluate((element) => {
      const label = element.querySelector('.mdc-tab__text-label') ?? element;
      return getComputedStyle(label).color;
    });
  expect(inactiveTabColor).not.toBe('rgba(0, 0, 0, 0.54)');
  await page.getByRole('tab', { name: 'Order entry' }).click();
  await expect(page.getByRole('heading', { name: 'Order entry' })).toBeVisible();
  await page.getByRole('tab', { name: 'Chart' }).click();
  await expect(page.getByRole('heading', { name: 'Chart' })).toBeVisible();
  await page.screenshot({
    path: 'test-artifacts/modern-mobile.png',
    fullPage: true,
  });
});
