// Derive an address card's identity (name + trust/self-settle badges) from its
// node Fields. Pure; consumed by AddressNode. Non-facilitator addresses (no
// facilitatorKnown field) get empty identity.

export function identityView(f) {
  const fields = f || {}
  let knownLabel = ''
  let knownTone = ''
  if (fields.facilitatorKnown === 'true') {
    knownLabel = 'known ✓'
    knownTone = 'known'
  } else if (fields.facilitatorKnown === 'false') {
    knownLabel = 'unknown ⚠'
    knownTone = 'unknown'
  }
  return {
    name: fields.entityLabel || '',
    knownLabel,
    knownTone,
    selfSettled: fields.selfSettled === 'true',
  }
}
