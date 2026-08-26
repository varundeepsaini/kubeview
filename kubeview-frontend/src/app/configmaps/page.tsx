"use client";
import ResourceList from "@/components/ResourceList"; import { api, ConfigMap } from "@/lib/api";
const columns=[{label:"Keys",render:(x:ConfigMap)=>x.keys.join(", ")||"<none>"},{label:"Age",render:(x:ConfigMap)=>x.age}];
export default function Page(){return <ResourceList title="ConfigMaps" fetcher={api.getConfigMaps} columns={columns}/>}
