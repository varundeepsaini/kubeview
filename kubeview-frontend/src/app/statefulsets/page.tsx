"use client";
import ResourceList from "@/components/ResourceList"; import { api, StatefulSet } from "@/lib/api";
const columns=[{label:"Ready",render:(x:StatefulSet)=>`${x.readyReplicas}/${x.desiredReplicas}`},{label:"Current",render:(x:StatefulSet)=>x.currentReplicas},{label:"Updated",render:(x:StatefulSet)=>x.updatedReplicas},{label:"Service",render:(x:StatefulSet)=>x.serviceName},{label:"Strategy",render:(x:StatefulSet)=>x.strategy},{label:"Age",render:(x:StatefulSet)=>x.age}];
export default function Page(){return <ResourceList title="StatefulSets" fetcher={api.getStatefulSets} columns={columns}/>}
