import Image from 'next/image';

interface Props {
  /** Pixel size used for both width and height (logo is square). */
  size: number;
  /** Use the 96px-optimized mark when small; otherwise the 512px asset. */
  variant?: 'mark' | 'full';
  /** Extra wrapper classes (positioning, margins). */
  className?: string;
  /** Pass `priority` for above-the-fold uses like the navbar / hero. */
  priority?: boolean;
}

/**
 * The kreise.berlin mark. Pure-black PNG on transparent background; we use
 * `dark:invert` so dark mode renders it as pure white without needing a
 * second asset. Stays crisp on retina because the source is 2-3× the
 * displayed CSS size.
 */
export function Logo({size, variant = 'mark', className, priority}: Props) {
  const src = variant === 'mark' ? '/logo-mark.png' : '/logo.png';
  return (
    <Image
      src={src}
      alt="kreise.berlin"
      width={size}
      height={size}
      priority={priority}
      className={['select-none dark:invert', className].filter(Boolean).join(' ')}
    />
  );
}
