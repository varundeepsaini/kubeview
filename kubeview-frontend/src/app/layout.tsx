import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";
import Sidebar from "@/components/Sidebar";
import { ClusterProvider } from "@/components/ClusterProvider";
import ClusterScope from "@/components/ClusterScope";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "KubeView - Kubernetes Dashboard",
  description: "Visual Kubernetes cluster management dashboard",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className="dark">
      <body
        className={`${geistSans.variable} ${geistMono.variable} antialiased font-sans`}
      >
        <ClusterProvider>
          <Sidebar />
          <ClusterScope>{children}</ClusterScope>
        </ClusterProvider>
      </body>
    </html>
  );
}
