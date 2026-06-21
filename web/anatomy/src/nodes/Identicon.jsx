import { identiconData } from '../identicon.js'

// Identicon renders the deterministic address fingerprint as a small SVG.
export default function Identicon({ address, size = 20 }) {
  const { hue, cells } = identiconData(address)
  const color = `hsl(${hue} 65% 62%)`
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 5 5"
      style={{ borderRadius: 3, background: '#0a0f0c', flexShrink: 0 }}
      aria-hidden="true"
    >
      {cells.flatMap((row, r) =>
        row.map((on, c) =>
          on ? <rect key={`${r}-${c}`} x={c} y={r} width="1" height="1" fill={color} /> : null,
        ),
      )}
    </svg>
  )
}
