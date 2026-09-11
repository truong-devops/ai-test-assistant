import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "AI Test Assistant · Document-driven testing",
  description: "Traceable test design from approved product documents, with code used only for automated execution.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
