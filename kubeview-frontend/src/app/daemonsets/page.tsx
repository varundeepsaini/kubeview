"use client";
import ResourceList from "@/components/ResourceList"; import { api, DaemonSet } from "@/lib/api";
const columns=[{label:"Desired",render:(x:DaemonSet)=>x.desired},{label:"Current",render:(x:DaemonSet)=>x.current},{label:"Ready",render:(x:DaemonSet)=>x.ready},{label:"Up-to-date",render:(x:DaemonSet)=>x.updated},{label:"Available",render:(x:DaemonSet)=>x.available},{label:"Age",render:(x:DaemonSet)=>x.age}];
export default function Page(){return <ResourceList title="DaemonSets" fetcher={api.getDaemonSets} columns={columns}/>}
