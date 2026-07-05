import { identiconData } from './identiconData'

export function Identicon({ address, size = 40 }: { address: string; size?: number }) {
  const { hue, cells } = identiconData(address)
  return (
    <span className="identicon" style={{ width: size, height: size, flexBasis: size }} aria-hidden="true">
      {cells.flatMap((row, r) =>
        row.map((on, c) => (
          <i key={`${r}-${c}`} style={on ? { background: `hsl(${hue} 55% ${45 + ((r + c) % 3) * 8}%)` } : undefined} />
        )),
      )}
    </span>
  )
}
