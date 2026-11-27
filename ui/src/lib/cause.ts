import type { ErrorCause, StepError } from './saga-types'

export const CAUSE_LABEL: Record<ErrorCause['kind'], string> = {
  credential: 'Credencial',
  timeout: 'Tempo esgotado',
  concurrency: 'Concorrência',
  infra: 'Infraestrutura',
  vendor: 'Fornecedor recusou',
  ours: 'Nosso lado',
  retries: 'Retentativas esgotadas',
  other: 'Sem classificação',
}

export const CAUSE_HELP: Record<ErrorCause['kind'], string> = {
  credential: 'Usuário, senha ou token recusado. Confira a credencial configurada para a integração.',
  timeout: 'A operação não terminou no prazo (recurso não ficou pronto, sessão expirou). Pode ser lentidão do fornecedor.',
  concurrency: 'Duas operações disputaram o mesmo recurso (lock, deadlock). Costuma passar ao repetir.',
  infra: 'Fila (RabbitMQ), banco, rede ou configuração do próprio serviço. Não é o fornecedor.',
  vendor: 'O fornecedor respondeu com erro. A resposta dele está na mensagem.',
  ours: 'Dados ou estado do nosso lado (registro não encontrado, estado inconsistente, validação).',
  retries: 'O worker desistiu depois de várias tentativas: a causa é o erro anterior a este.',
  other: 'Não deu para classificar pela mensagem.',
}

/** "Rubrik recusou", "Credencial (JCStack)", "Nosso lado"... */
export function causeText(c: ErrorCause) {
  if (c.kind === 'vendor') return c.vendor ? `${c.vendor} recusou` : CAUSE_LABEL.vendor
  if (c.vendor && (c.kind === 'credential' || c.kind === 'timeout' || c.kind === 'concurrency')) return `${CAUSE_LABEL[c.kind]} (${c.vendor})`
  return CAUSE_LABEL[c.kind]
}

/** A causa mais informativa de uma etapa: retentativas e "sem classificação" só se não houver outra. */
export function mainCause(errors: StepError[] | undefined): ErrorCause | undefined {
  const cs = (errors ?? []).map((e) => e.cause).filter((c): c is ErrorCause => !!c)
  return cs.find((c) => c.kind !== 'retries' && c.kind !== 'other') ?? cs[0]
}
