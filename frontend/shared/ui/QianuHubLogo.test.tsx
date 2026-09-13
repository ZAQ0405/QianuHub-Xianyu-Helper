import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, test } from 'vitest';
import QianuHubLogo, { QianuHubBrandIcon } from './QianuHubLogo';

describe('QianuHubLogo', /* 当前回调处理用户交互或异步状态变化。 */ () => {
  test('uses the provided QianuHub brand image', /* 当前回调处理用户交互或异步状态变化。 */ () => {
    // html 渲染后的 HTML。
    const html = renderToStaticMarkup(<QianuHubLogo />);
    expect(html).toContain('src="/static/qianuhub-logo.png"');
    expect(html).toContain('alt="QianuHub"');
  });

  test('renders the provided image as a square mark', () => {
    const markHtml = renderToStaticMarkup(<QianuHubBrandIcon />);
    expect(markHtml).toContain('qianuhub-logo.png');
    expect(markHtml).toContain('object-contain');
  });
});
