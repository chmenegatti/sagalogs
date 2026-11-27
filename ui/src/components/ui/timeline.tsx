// Base: Aceternity UI Timeline (ui.aceternity.com/registry/timeline.json).
// Adaptado: sem cabeçalho fixo, títulos menores, cores do tema.
import { useScroll, useTransform, motion } from 'motion/react'
import React, { useEffect, useRef, useState } from 'react'

export interface TimelineEntry {
  id: string
  title: React.ReactNode
  subtitle?: React.ReactNode
  content: React.ReactNode
}

export const Timeline = ({ data }: { data: TimelineEntry[] }) => {
  const ref = useRef<HTMLDivElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const [height, setHeight] = useState(0)

  useEffect(() => {
    if (!ref.current) return
    const ro = new ResizeObserver(() => setHeight(ref.current!.getBoundingClientRect().height))
    ro.observe(ref.current)
    return () => ro.disconnect()
  }, [])

  const { scrollYProgress } = useScroll({ target: containerRef, offset: ['start 20%', 'end 60%'] })
  const heightTransform = useTransform(scrollYProgress, [0, 1], [0, height])
  const opacityTransform = useTransform(scrollYProgress, [0, 0.05], [0, 1])

  return (
    <div className="w-full" ref={containerRef}>
      <div ref={ref} className="relative pb-6">
        {data.map((item) => (
          <div key={item.id} className="flex justify-start pt-8 first:pt-2 md:gap-8">
            <div className="sticky top-24 z-20 flex flex-col items-center self-start md:w-48 md:flex-row lg:w-56">
              <div className="absolute left-0 flex size-10 items-center justify-center rounded-full bg-paper">
                <div className="size-3.5 rounded-full border-2 border-route bg-sheet" />
              </div>
              <div className="hidden pl-14 md:block">
                <div className="font-mono text-[15px] font-medium text-ink">{item.title}</div>
                {item.subtitle && <div className="mt-0.5 text-[12px] text-ink-3">{item.subtitle}</div>}
              </div>
            </div>
            <div className="relative w-full pr-1 pl-14 md:pl-0">
              <div className="mb-2 md:hidden">
                <div className="font-mono text-[14px] font-medium text-ink">{item.title}</div>
                {item.subtitle && <div className="text-[12px] text-ink-3">{item.subtitle}</div>}
              </div>
              {item.content}
            </div>
          </div>
        ))}
        <div
          style={{ height: height + 'px' }}
          className="absolute top-0 left-5 w-[2px] overflow-hidden bg-rule [mask-image:linear-gradient(to_bottom,transparent_0%,black_4%,black_96%,transparent_100%)]"
        >
          <motion.div
            style={{ height: heightTransform, opacity: opacityTransform }}
            className="absolute inset-x-0 top-0 w-[2px] rounded-full bg-gradient-to-b from-route/0 via-route to-route"
          />
        </div>
      </div>
    </div>
  )
}
