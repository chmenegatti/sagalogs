import {
  Archive,
  Cloud,
  Cpu,
  Monitor,
  UserRound,
  Database,
  HardDrive,
  Network,
  DoorOpen,
  Flame,
  Inbox,
  type LucideIcon,
  Server,
  Shield,
  Workflow,
} from 'lucide-react'
import type { AppInfo, Step } from '@/lib/saga-types'

export function iconFor(step: Pick<Step, 'kind' | 'app'>, app?: AppInfo): LucideIcon {
  if (step.kind === 'gateway') return DoorOpen
  if (step.kind === 'outbox') return Inbox
  if (step.kind === 'orchestrator') return Workflow
  if (app?.final) return Database
  if (app?.vendor === 'fortinet') return Shield
  if (app?.vendor === 'paloalto') return Flame
  if (app?.vendor === 'nsxt') return Network
  if (app?.vendor === 'ontap') return HardDrive
  if (app?.vendor === 'rubrik') return Archive
  if (app?.vendor === 'citrix') return Monitor
  if (app?.vendor === 'entraid') return UserRound
  if (app?.vendor === 'vmware') return Cpu
  if (app?.role === 'worker') return Cloud
  return Server
}

export const VENDOR_LABEL: Record<string, string> = {
  fortinet: 'Fortinet',
  paloalto: 'Palo Alto',
  nsxt: 'NSX-T',
  ontap: 'NetApp ONTAP',
  rubrik: 'Rubrik',
  citrix: 'Citrix',
  entraid: 'Entra ID',
  vmware: 'VMware',
  interno: 'Interno',
}

export const ROLE_LABEL: Record<AppInfo['role'], string> = {
  gateway: 'Gateway',
  orchestrator: 'Orquestrador',
  worker: 'Worker',
}

/** Rótulo curto para estação: tira o prefixo do fornecedor. */
export function shortLabel(step: Step): string {
  return step.label.replace(/^(Fortinet|Palo Alto|Citrix|NSX-T|vSphere|Entra ID) /, '').replace(/ · execução (\d+)/, ' #$1')
}
