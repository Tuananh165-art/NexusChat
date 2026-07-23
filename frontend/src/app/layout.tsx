import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "NexusChat",
  description: "Modern real-time chat platform",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <head>
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
        <link
          href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&display=swap"
          rel="stylesheet"
        />
      </head>
      <body className="antialiased font-[Inter,sans-serif] text-text-primary bg-[#0a0a1a]">
        {children}
      </body>
    </html>
  );
}
