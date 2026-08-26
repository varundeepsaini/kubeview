"use client";
import { useState } from "react";
import ResourceList from "@/components/ResourceList";
import { api, Secret } from "@/lib/api";

function SecretData({ secret }: { secret: Secret }) {
  const [values, setValues] = useState<Record<string,string> | null>(null);
  const [error, setError] = useState("");
  const reveal = async () => { try { setValues((await api.getSecret(secret.namespace, secret.name)).values); setError(""); } catch (e) { setError(e instanceof Error ? e.message : "Unable to reveal") } };
  if (values) return <div className="space-y-1">{Object.entries(values).map(([key,value])=><div key={key}><span className="font-medium text-foreground">{key}:</span> <code className="break-all">{value}</code></div>)}<button className="text-accent hover:underline" onClick={()=>setValues(null)}>Hide</button></div>;
  return <div>{Object.entries(secret.dataLengths).map(([key,length])=><div key={key}>{key} <span className="text-xs">({length} bytes)</span></div>)}<button className="mt-1 text-accent hover:underline" onClick={reveal}>Reveal</button>{error&&<p className="text-red-400">{error}</p>}</div>;
}
const columns=[{label:"Type",render:(x:Secret)=>x.type},{label:"Data",render:(x:Secret)=><SecretData secret={x}/>},{label:"Age",render:(x:Secret)=>x.age}];
export default function Page(){return <ResourceList title="Secrets" fetcher={api.getSecrets} columns={columns}/>}
