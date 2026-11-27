import { Dialog } from 'radix-ui'
import { Check, ChevronsDownUp, ChevronsUpDown, Copy, Download, X } from 'lucide-react'
import { useState } from 'react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { copyText } from '@/lib/clipboard'
import type { Step } from '@/lib/saga-types'
import { cn } from '@/lib/utils'
import { JsonDiff, JsonTree } from './json-tree'

export type PayloadTab = 'in' | 'out' | 'diff'

/** Abas Entrada / Saída / O que mudou. Usada no painel da etapa e no modal. */
export function PayloadTabs({
  step,
  tab,
  onTab,
  className,
  openDepth,
  full,
  toolbar,
}: {
  step: Step
  tab: PayloadTab
  onTab: (t: PayloadTab) => void
  className?: string
  openDepth?: number
  full?: boolean
  toolbar?: React.ReactNode
}) {
  const hasIn = step.payloadIn !== undefined
  const hasOut = step.payloadOut !== undefined
  return (
    <Tabs value={tab} onValueChange={(v) => onTab(v as PayloadTab)} className="min-h-0 flex-1">
      <div className="flex flex-wrap items-center gap-2">
        <TabsList className="h-8">
          <TabsTrigger value="in" disabled={!hasIn} className="text-[12px]">
            Entrada
          </TabsTrigger>
          <TabsTrigger value="out" disabled={!hasOut} className="text-[12px]">
            Saída
          </TabsTrigger>
          <TabsTrigger value="diff" disabled={!hasIn || !hasOut} className="text-[12px]">
            O que mudou
          </TabsTrigger>
        </TabsList>
        {toolbar}
      </div>
      <div className={cn('mt-3 overflow-auto rounded-md border border-rule p-2 font-mono leading-relaxed', className)}>
        <TabsContent value="in">
          <JsonTree value={step.payloadIn} openDepth={openDepth} full={full} />
        </TabsContent>
        <TabsContent value="out">
          <JsonTree value={step.payloadOut} openDepth={openDepth} full={full} />
        </TabsContent>
        <TabsContent value="diff">
          <JsonDiff before={step.payloadIn} after={step.payloadOut} />
        </TabsContent>
      </div>
    </Tabs>
  )
}

/** Payload da etapa em tela grande: árvore completa, sem cortar strings, com copiar e baixar. */
export function PayloadModal({
  step,
  title,
  subtitle,
  tab,
  onTab,
  open,
  onOpenChange,
}: {
  step: Step
  title: string
  subtitle: string
  tab: PayloadTab
  onTab: (t: PayloadTab) => void
  open: boolean
  onOpenChange: (o: boolean) => void
}) {
  // gen muda a key da árvore: expandir/recolher tudo recria os nós com o novo openDepth
  const [all, setAll] = useState({ open: false, gen: 0 })
  const [copied, setCopied] = useState(false)
  const value = tab === 'in' ? step.payloadIn : tab === 'out' ? step.payloadOut : undefined
  const json = value === undefined ? '' : JSON.stringify(value, null, 2)

  const copy = async () => {
    if (await copyText(json)) {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    }
  }
  const download = () => {
    const url = URL.createObjectURL(new Blob([json], { type: 'application/json' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `${step.app}-${tab === 'in' ? 'entrada' : 'saida'}.json`
    a.click()
    URL.revokeObjectURL(url)
  }

  const btn =
    'inline-flex h-8 items-center gap-1.5 rounded-md border border-rule px-2.5 text-[12px] text-ink-2 hover:bg-paper hover:text-ink disabled:pointer-events-none disabled:opacity-40'

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-[rgb(8_12_22/0.55)] backdrop-blur-[2px]" />
        <Dialog.Content
          className="fixed inset-3 z-50 flex flex-col rounded-lg border border-rule bg-sheet text-ink shadow-2xl outline-none md:inset-8"
          aria-describedby={undefined}
        >
          <header className="flex items-start gap-3 border-b border-rule px-5 py-4">
            <div className="min-w-0">
              <Dialog.Title className="text-[16px] font-semibold">Payload: {title}</Dialog.Title>
              <p className="mt-0.5 truncate font-mono text-[12px] text-ink-2">{subtitle}</p>
            </div>
            <Dialog.Close className="ml-auto rounded-md p-1.5 text-ink-3 hover:bg-paper hover:text-ink" aria-label="Fechar">
              <X className="size-5" />
            </Dialog.Close>
          </header>
          <div className="flex min-h-0 flex-1 flex-col px-5 pt-4 pb-5">
            <PayloadTabs
              key={all.gen}
              step={step}
              tab={tab}
              onTab={onTab}
              openDepth={all.open ? Infinity : 3}
              full
              className="h-[calc(100%-2.75rem)] text-[13px]"
              toolbar={
                <div className="ml-auto flex flex-wrap gap-2">
                  <button
                    type="button"
                    className={btn}
                    disabled={tab === 'diff'}
                    onClick={() => setAll((s) => ({ open: !s.open, gen: s.gen + 1 }))}
                  >
                    {all.open ? <ChevronsDownUp className="size-3.5" /> : <ChevronsUpDown className="size-3.5" />}
                    {all.open ? 'Recolher tudo' : 'Expandir tudo'}
                  </button>
                  <button type="button" className={btn} disabled={!json} onClick={copy}>
                    {copied ? <Check className="size-3.5 text-ok" /> : <Copy className="size-3.5" />}
                    {copied ? 'Copiado' : 'Copiar JSON'}
                  </button>
                  <button type="button" className={btn} disabled={!json} onClick={download}>
                    <Download className="size-3.5" />
                    Baixar .json
                  </button>
                </div>
              }
            />
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
