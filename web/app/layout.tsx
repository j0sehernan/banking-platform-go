import './globals.css';
import type { Metadata } from 'next';
import { ActivityPanel } from '@/components/ActivityPanel';

export const metadata: Metadata = {
  title: 'Banking Platform',
  description: 'Event-driven banking platform demo with Go + Kafka + LLM',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>
        <div className="flex min-h-screen">
          <main className="flex-1 overflow-y-auto">{children}</main>
          <ActivityPanel />
        </div>
      </body>
    </html>
  );
}
