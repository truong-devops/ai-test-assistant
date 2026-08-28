import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Test Review · AI Test Assistant",
  description: "Traceable human review for AI-generated Go tests.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
