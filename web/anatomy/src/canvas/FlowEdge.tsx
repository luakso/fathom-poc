import { BaseEdge, EdgeLabelRenderer, getBezierPath, type EdgeProps, type Edge } from '@xyflow/react'
import type { FlowEdgeData } from './graph'

export function FlowEdge(props: EdgeProps<Edge<FlowEdgeData>>) {
  const data = props.data ?? {}
  const [path, labelX, labelY] = getBezierPath(props)
  const share = Math.min(data.share ?? 0, 1)
  const width = data.verb ? 2 : data.ghost ? 1.3 : 1.5 + 4.5 * share
  const stroke = data.verb ? '#f2c14e' : data.ghost ? 'rgba(143,160,152,.5)' : '#3dd68c'
  const dash = data.verb || data.ghost ? '7 5' : undefined
  const glow = Boolean(data.primary) || Boolean(props.selected)
  const labelCls = ['edge-label', data.primary ? 'primary' : '', data.verb ? 'verb' : '', data.ghost ? 'dim' : ''].join(' ').trim()
  return (
    <>
      {glow && (
        <path d={path} fill="none" stroke={stroke} strokeWidth={width * 3} opacity={0.35} style={{ filter: 'blur(4px)' }} />
      )}
      <BaseEdge id={props.id} path={path} style={{ stroke, strokeWidth: width, strokeDasharray: dash }} />
      {data.label && (
        <EdgeLabelRenderer>
          <div className={labelCls} style={{ position: 'absolute', transform: `translate(-50%,-50%) translate(${labelX}px,${labelY}px)` }}>
            {data.label}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  )
}
