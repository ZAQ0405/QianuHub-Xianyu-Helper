import React from 'react';

const QIANUHUB_LOGO_SRC = '/static/qianuhub-logo.png';

interface QianuHubLogoProps {
  /** className 表示完整品牌图片的附加 CSS 类名。 */ className?: string;
}

interface BrandIconProps {
  /** sizeClass 指定品牌图标尺寸相关的 Tailwind 类名。 */ sizeClass?: string;
  /** logoClassName 表示品牌图片的 CSS 类名。 */ logoClassName?: string;
}

// QianuHubBrandIcon 使用 QianuHub 1:1 Logo 图片，用于侧栏折叠态和登录页。
export const QianuHubBrandIcon: React.FC<BrandIconProps> = ({
  sizeClass = 'h-12 w-12',
  logoClassName = 'transition-transform duration-300 group-hover:scale-105',
}) => (
  <span className={`relative z-10 block overflow-hidden rounded-xl bg-slate-950 ${sizeClass}`} role="img" aria-label="QianuHub">
    <img
      src={QIANUHUB_LOGO_SRC}
      alt=""
      className={`h-full w-full object-contain ${logoClassName}`}
    />
  </span>
);

const QianuHubLogo: React.FC<QianuHubLogoProps> = ({ className = 'h-full w-full' }) => (
  <img src={QIANUHUB_LOGO_SRC} alt="QianuHub" className={className} />
);

export default QianuHubLogo;
