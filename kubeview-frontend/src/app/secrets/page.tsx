"use client";
import ResourceList from "@/components/ResourceList";
import { api, Secret } from "@/lib/api";

function SecretData({ secret }: { secret: Secret }) {
  return <div>{Object.entries(secret.dataLengths).map(([key,length])=><div key={key}>{key} <span className="text-xs">({length} bytes)</span></div>)}</div>;
}
const columns=[{label:"Type",render:(x:Secret)=>x.type},{label:"Data",render:(x:Secret)=><SecretData secret={x}/>},{label:"Age",render:(x:Secret)=>x.age}];
export default function Page(){return <ResourceList title="Secrets" fetcher={api.getSecrets} columns={columns}/>}
