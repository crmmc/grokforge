import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import rehypeHighlight from 'rehype-highlight'
import 'highlight.js/styles/github-dark.css'

const components: Components = {
  pre: ({ children, ...props }) => (
    <pre className="overflow-x-auto rounded-lg bg-[#0d1117] p-4 my-2" {...props}>{children}</pre>
  ),
  code: ({ className, children, ...props }) => {
    // Block code (inside <pre>) has a language-* className from rehype-highlight
    if (className?.includes('language-') || className?.includes('hljs')) {
      return <code className={className} {...props}>{children}</code>
    }
    // Inline code
    return <code className="bg-[rgba(0,0,0,0.04)] px-1.5 py-0.5 rounded text-sm" {...props}>{children}</code>
  },
  a: ({ children, ...props }) => (
    <a className="text-primary underline" target="_blank" rel="noopener noreferrer" {...props}>{children}</a>
  ),
  p: ({ children, ...props }) => (
    <p className="my-1 leading-relaxed" {...props}>{children}</p>
  ),
  ul: ({ children, ...props }) => (
    <ul className="my-1 ml-4 list-disc" {...props}>{children}</ul>
  ),
  ol: ({ children, ...props }) => (
    <ol className="my-1 ml-4 list-decimal" {...props}>{children}</ol>
  ),
  h1: ({ children, ...props }) => (
    <h1 className="text-2xl font-semibold my-2" {...props}>{children}</h1>
  ),
  h2: ({ children, ...props }) => (
    <h2 className="text-xl font-semibold my-2" {...props}>{children}</h2>
  ),
  h3: ({ children, ...props }) => (
    <h3 className="text-lg font-semibold my-2" {...props}>{children}</h3>
  ),
  h4: ({ children, ...props }) => (
    <h4 className="text-base font-semibold my-2" {...props}>{children}</h4>
  ),
}

export function MarkdownRenderer({ content, className }: { content: string; className?: string }) {
  return (
    <div className={className}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]} components={components}>
        {content}
      </ReactMarkdown>
    </div>
  )
}
