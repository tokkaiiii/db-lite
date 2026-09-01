import type { DBKind } from '../api/types'

const KIND_CLASS: Record<DBKind, string> = {
  mysql: 'badge-kind-mysql',
  postgres: 'badge-kind-postgres',
  mssql: 'badge-kind-mssql',
  oracle: 'badge-kind-oracle',
}

export function DbKindBadge({ kind }: { kind: DBKind }) {
  return <span className={`badge badge-kind ${KIND_CLASS[kind]}`}>{kind}</span>
}
