"use client";
import ResourceList from "@/components/ResourceList"; import { api, Ingress } from "@/lib/api";
const columns=[{label:"Class",render:(x:Ingress)=>x.class},{label:"Hosts / paths",render:(x:Ingress)=>x.rules.map(r=>`${r.host||"*"}${r.path} -> ${r.service}:${r.port}`).join(", ")||"<none>"},{label:"Address",render:(x:Ingress)=>x.addresses.join(", ")||"<pending>"},{label:"Age",render:(x:Ingress)=>x.age}];
export default function Page(){return <ResourceList title="Ingresses" fetcher={api.getIngresses} columns={columns}/>}
